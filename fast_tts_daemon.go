//go:build tts_experimental
// +build tts_experimental

package main

import (
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
	defaultSpeaker = "p245"
	dragThreshold  = 3 * time.Second
	serverPort     = ":8090"
)

type SpeakRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
}

type SpeakResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type TTSChunk struct {
	Index     int
	Text      string
	FilePath  string
	Ready     chan bool
	StartTime time.Time
	EndTime   time.Time
	Error     error
}

type FastTTSDaemon struct {
	client    *http.Client
	outputDir string
}

func NewFastTTSDaemon() *FastTTSDaemon {
	// Simple, fast HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        20,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     60 * time.Second,
			DisableKeepAlives:   false,
		},
	}

	outputDir := "/tmp/fast_tts_daemon"
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)

	return &FastTTSDaemon{
		client:    client,
		outputDir: outputDir,
	}
}

func (d *FastTTSDaemon) downloadTTS(chunk *TTSChunk, speaker string, wg *sync.WaitGroup) {
	defer wg.Done()
	
	chunk.StartTime = time.Now()
	encodedText := url.QueryEscape(chunk.Text)
	requestURL := fmt.Sprintf("%s?text=%s&speaker_id=%s", ttsURL, encodedText, speaker)
	
	resp, err := d.client.Get(requestURL)
	if err != nil {
		chunk.Error = err
		close(chunk.Ready)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		chunk.Error = fmt.Errorf("HTTP %d", resp.StatusCode)
		close(chunk.Ready)
		return
	}
	
	file, err := os.Create(chunk.FilePath)
	if err != nil {
		chunk.Error = err
		close(chunk.Ready)
		return
	}
	defer file.Close()
	
	_, err = io.Copy(file, resp.Body)
	chunk.Error = err
	chunk.EndTime = time.Now()
	
	// Log drag detection
	duration := chunk.EndTime.Sub(chunk.StartTime)
	if duration > dragThreshold {
		log.Printf("DRAG DETECTED: Chunk %d took %v (threshold: %v)", chunk.Index, duration, dragThreshold)
	}
	
	close(chunk.Ready)
}

func (d *FastTTSDaemon) playAudio(filePath string) error {
	cmd := exec.Command("aplay", filePath)
	cmd.Stderr = nil
	return cmd.Run()
}

func (d *FastTTSDaemon) processSpeak(sentences []string, speaker string) {
	if speaker == "" {
		speaker = defaultSpeaker
	}
	
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	
	log.Printf("Processing %d sentences concurrently...", len(sentences))
	
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
	
	// Launch all TTS requests concurrently
	var wg sync.WaitGroup
	ttsStart := time.Now()
	
	for _, chunk := range chunks {
		wg.Add(1)
		go d.downloadTTS(chunk, speaker, &wg)
	}
	
	// Stream playback as chunks become ready
	go func() {
		for _, chunk := range chunks {
			// Wait for this chunk to be ready
			<-chunk.Ready
			
			if chunk.Error != nil {
				log.Printf("ERROR: Chunk %d failed: %v", chunk.Index, chunk.Error)
				continue
			}
			
			// Calculate time to ready
			timeToReady := chunk.EndTime.Sub(ttsStart)
			log.Printf("Playing chunk %d (ready in %v)", chunk.Index, timeToReady)
			
			// Play immediately
			if err := d.playAudio(chunk.FilePath); err != nil {
				log.Printf("ERROR: Failed to play chunk %d: %v", chunk.Index, err)
			}
			
			// Clean up audio file after playing
			os.Remove(chunk.FilePath)
		}
	}()
	
	// Wait for all downloads to complete in background
	go func() {
		wg.Wait()
		log.Printf("All chunks completed")
	}()
}

func (d *FastTTSDaemon) handleSpeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req SpeakRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	if len(req.Sentences) == 0 {
		http.Error(w, "No sentences provided", http.StatusBadRequest)
		return
	}
	
	// Process in background and respond immediately
	go d.processSpeak(req.Sentences, req.Speaker)
	
	// Immediate response
	response := SpeakResponse{
		Success: true,
		Message: fmt.Sprintf("Processing %d sentences", len(req.Sentences)),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (d *FastTTSDaemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Quick health check
	status := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func main() {
	daemon := NewFastTTSDaemon()
	
	// Setup routes
	http.HandleFunc("/speak", daemon.handleSpeak)
	http.HandleFunc("/health", daemon.handleHealth)
	
	// Graceful shutdown
	server := &http.Server{
		Addr:    serverPort,
		Handler: nil,
	}
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		log.Println("Shutting down fast TTS daemon...")
		server.Close()
		os.RemoveAll(daemon.outputDir)
	}()
	
	log.Printf("Fast TTS daemon starting on port %s", serverPort)
	log.Println("Endpoints: /speak, /health")
	
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
	
	log.Println("Fast TTS daemon shut down")
}