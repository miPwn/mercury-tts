//go:build tts_experimental
// +build tts_experimental

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const (
	ttsURL         = "http://localhost:5002/api/tts"
	defaultSpeaker = "p280" // Deeper voice
	maxRetries     = 3
	serverPort     = ":8080"
)

type TTSRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
}

type TTSResponse struct {
	Success      bool          `json:"success"`
	Message      string        `json:"message,omitempty"`
	Performance  *Performance  `json:"performance,omitempty"`
	AudioFiles   []string      `json:"audio_files,omitempty"`
}

type Performance struct {
	TotalDuration     time.Duration `json:"total_duration_ms"`
	TTSGeneration     time.Duration `json:"tts_generation_ms"`
	FirstChunkReady   time.Duration `json:"first_chunk_ready_ms"`
	DNSLookup         time.Duration `json:"dns_lookup_ms"`
	TCPConnect        time.Duration `json:"tcp_connect_ms"`
	TLSHandshake      time.Duration `json:"tls_handshake_ms"`
	ServerProcessing  time.Duration `json:"server_processing_ms"`
	ContentTransfer   time.Duration `json:"content_transfer_ms"`
	ChunksGenerated   int           `json:"chunks_generated"`
	ConcurrentJobs    int           `json:"concurrent_jobs"`
	ConnectionsReused int           `json:"connections_reused"`
	RetryAttempts     int           `json:"retry_attempts"`
}

type TTSChunk struct {
	Index     int
	Text      string
	FilePath  string
	Ready     chan bool
	Error     error
	StartTime time.Time
	EndTime   time.Time
}

type TTSDaemon struct {
	client    *http.Client
	outputDir string
	mu        sync.RWMutex
	stats     map[string]*Performance
}

func NewTTSDaemon() *TTSDaemon {
	// Optimized HTTP client with aggressive connection pooling
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     120 * time.Second,
			DisableKeepAlives:   false,
			MaxConnsPerHost:     20,
		},
	}

	outputDir := "/tmp/tts_daemon_cache"
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)

	daemon := &TTSDaemon{
		client:    client,
		outputDir: outputDir,
		stats:     make(map[string]*Performance),
	}

	// Pre-warm the TTS service on startup
	daemon.preWarmService()
	
	return daemon
}

func (d *TTSDaemon) preWarmService() {
	log.Println("Pre-warming TTS service...")
	start := time.Now()
	
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s?text=warmup&speaker_id=%s", ttsURL, defaultSpeaker), nil)
	resp, err := d.client.Do(req)
	if err == nil && resp != nil {
		resp.Body.Close()
		log.Printf("TTS service warmed up in %v", time.Since(start))
	} else {
		log.Printf("TTS warmup failed: %v", err)
	}
}

func (d *TTSDaemon) generateTTS(chunk *TTSChunk, wg *sync.WaitGroup, speaker string) {
	defer wg.Done()
	
	chunk.StartTime = time.Now()
	encodedText := url.QueryEscape(chunk.Text)
	requestURL := fmt.Sprintf("%s?text=%s&speaker_id=%s", ttsURL, encodedText, speaker)
	
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = d.downloadAudio(requestURL, chunk.FilePath)
		if err == nil {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}
	
	chunk.EndTime = time.Now()
	chunk.Error = err
	close(chunk.Ready)
}

func (d *TTSDaemon) downloadAudio(requestURL, filePath string) error {
	resp, err := d.client.Get(requestURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

func (d *TTSDaemon) streamTTS(sentences []string, speaker string) (*TTSResponse, error) {
	if speaker == "" {
		speaker = defaultSpeaker
	}
	
	startTime := time.Now()
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	
	// Create chunks
	chunks := make([]*TTSChunk, len(sentences))
	for i, sentence := range sentences {
		chunks[i] = &TTSChunk{
			Index:    i + 1,
			Text:     sentence,
			FilePath: filepath.Join(d.outputDir, fmt.Sprintf("%s_%d.wav", timestamp, i+1)),
			Ready:    make(chan bool),
		}
	}
	
	// Start all TTS generation goroutines
	var wg sync.WaitGroup
	ttsStart := time.Now()
	
	for _, chunk := range chunks {
		wg.Add(1)
		go d.generateTTS(chunk, &wg, speaker)
	}
	
	// Wait for first chunk to complete (for first-chunk latency measurement)
	var firstChunkTime time.Time
	go func() {
		<-chunks[0].Ready
		firstChunkTime = time.Now()
	}()
	
	// Wait for all generation to complete
	wg.Wait()
	ttsEnd := time.Now()
	
	// Collect audio files and check for errors
	var audioFiles []string
	var errorCount int
	
	for _, chunk := range chunks {
		if chunk.Error != nil {
			errorCount++
			log.Printf("Chunk %d error: %v", chunk.Index, chunk.Error)
		} else {
			audioFiles = append(audioFiles, chunk.FilePath)
		}
	}
	
	// Calculate performance metrics
	perf := &Performance{
		TotalDuration:   time.Since(startTime),
		TTSGeneration:   ttsEnd.Sub(ttsStart),
		FirstChunkReady: firstChunkTime.Sub(ttsStart),
		ChunksGenerated: len(audioFiles),
		ConcurrentJobs:  len(sentences),
	}
	
	// Store stats
	d.mu.Lock()
	d.stats[timestamp] = perf
	d.mu.Unlock()
	
	if errorCount > 0 {
		return &TTSResponse{
			Success:     false,
			Message:     fmt.Sprintf("%d chunks failed to generate", errorCount),
			Performance: perf,
			AudioFiles:  audioFiles,
		}, nil
	}
	
	return &TTSResponse{
		Success:     true,
		Performance: perf,
		AudioFiles:  audioFiles,
	}, nil
}

func (d *TTSDaemon) handleTTSRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req TTSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	if len(req.Sentences) == 0 {
		http.Error(w, "No sentences provided", http.StatusBadRequest)
		return
	}
	
	log.Printf("Processing TTS request: %d sentences, speaker: %s", len(req.Sentences), req.Speaker)
	
	response, err := d.streamTTS(req.Sentences, req.Speaker)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (d *TTSDaemon) handlePlayAudio(w http.ResponseWriter, r *http.Request) {
	audioFile := r.URL.Query().Get("file")
	if audioFile == "" {
		http.Error(w, "No audio file specified", http.StatusBadRequest)
		return
	}
	
	// Security check - ensure file is in our output directory
	if !filepath.HasPrefix(audioFile, d.outputDir) {
		http.Error(w, "Invalid file path", http.StatusForbidden)
		return
	}
	
	// Play the audio file
	cmd := exec.Command("aplay", audioFile)
	if err := cmd.Run(); err != nil {
		log.Printf("Error playing audio: %v", err)
		http.Error(w, "Failed to play audio", http.StatusInternalServerError)
		return
	}
	
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Audio played successfully"))
}

func (d *TTSDaemon) handleStats(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.stats)
}

func (d *TTSDaemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Test TTS service connectivity
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s?text=health&speaker_id=%s", ttsURL, defaultSpeaker), nil)
	resp, err := d.client.Do(req)
	
	status := map[string]interface{}{
		"daemon":       "healthy",
		"tts_service":  "unknown",
		"timestamp":    time.Now().Unix(),
	}
	
	if err == nil && resp != nil {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			status["tts_service"] = "healthy"
		} else {
			status["tts_service"] = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	} else {
		status["tts_service"] = fmt.Sprintf("error: %v", err)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func main() {
	daemon := NewTTSDaemon()
	
	// Setup HTTP routes
	http.HandleFunc("/tts", daemon.handleTTSRequest)
	http.HandleFunc("/play", daemon.handlePlayAudio)
	http.HandleFunc("/stats", daemon.handleStats)
	http.HandleFunc("/health", daemon.handleHealth)
	
	// Graceful shutdown
	server := &http.Server{
		Addr:    serverPort,
		Handler: nil,
	}
	
	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		log.Println("Shutting down TTS daemon...")
		
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		server.Shutdown(ctx)
		os.RemoveAll(daemon.outputDir)
	}()
	
	log.Printf("TTS daemon starting on port %s", serverPort)
	log.Printf("Endpoints: /tts, /play, /stats, /health")
	
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
	
	log.Println("TTS daemon shut down gracefully")
}