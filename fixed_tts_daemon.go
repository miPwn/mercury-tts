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
	ttsURL         = "http://192.168.1.106:5002/api/tts"
	defaultSpeaker = "p245"
	dragThreshold  = 3 * time.Second
	serverPort     = ":8091"
	maxConcurrent  = 2 // Limit concurrent TTS requests
)

type SpeakRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
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

type FixedTTSDaemon struct {
	client        *http.Client
	outputDir     string
	playbackReady chan bool
	throttle      chan struct{} // Semaphore for rate limiting
}

func NewFixedTTSDaemon() *FixedTTSDaemon {
	outputDir := "/tmp/fixed_tts_daemon"
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)

	daemon := &FixedTTSDaemon{
		client: &http.Client{
			Timeout: 15 * time.Second, // Longer timeout for TTS processing
			Transport: &http.Transport{
				MaxIdleConns:          10,
				MaxIdleConnsPerHost:   3,  // Reduced concurrent connections
				IdleConnTimeout:       60 * time.Second,
				DisableKeepAlives:     false,
				ResponseHeaderTimeout: 10 * time.Second,
			},
		},
		outputDir:     outputDir,
		playbackReady: make(chan bool, 1),
		throttle:      make(chan struct{}, maxConcurrent), // Limit concurrent requests
	}

	// Pre-warm system
	daemon.preWarmSystem()
	
	return daemon
}

func (d *FixedTTSDaemon) preWarmSystem() {
	log.Println("Pre-warming TTS system...")
	
	// Simple warmup - just one request
	resp, err := d.client.Get(fmt.Sprintf("%s?text=warmup&speaker_id=%s", ttsURL, defaultSpeaker))
	if err == nil && resp != nil {
		resp.Body.Close()
		log.Println("TTS system warmed up")
	}
	
	// Prepare audio system
	d.playbackReady <- true
	log.Println("Audio system ready")
}

func (d *FixedTTSDaemon) downloadTTS(chunk *TTSChunk, speaker string, wg *sync.WaitGroup) {
	defer wg.Done()
	
	// Acquire throttle semaphore
	d.throttle <- struct{}{}
	defer func() { <-d.throttle }()
	
	chunk.StartTime = time.Now()
	encodedText := url.QueryEscape(chunk.Text)
	requestURL := fmt.Sprintf("%s?text=%s&speaker_id=%s", ttsURL, encodedText, speaker)
	
	var err error
	maxRetries := 3
	
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			log.Printf("Retrying chunk %d (attempt %d)", chunk.Index, attempt+1)
		}
		
		resp, err := d.client.Get(requestURL)
		if err != nil {
			log.Printf("Chunk %d attempt %d: network error: %v", chunk.Index, attempt+1, err)
			continue
		}
		
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			err = fmt.Errorf("HTTP %d", resp.StatusCode)
			log.Printf("Chunk %d attempt %d: HTTP error: %v", chunk.Index, attempt+1, err)
			continue
		}
		
		// Success - save the audio
		file, createErr := os.Create(chunk.FilePath)
		if createErr != nil {
			resp.Body.Close()
			err = createErr
			continue
		}
		
		_, copyErr := io.Copy(file, resp.Body)
		file.Close()
		resp.Body.Close()
		
		if copyErr != nil {
			err = copyErr
			continue
		}
		
		// Success!
		err = nil
		break
	}
	
	chunk.Error = err
	chunk.EndTime = time.Now()
	
	duration := chunk.EndTime.Sub(chunk.StartTime)
	if duration > dragThreshold {
		log.Printf("DRAG: Chunk %d took %v", chunk.Index, duration)
	}
	
	close(chunk.Ready)
}

func (d *FixedTTSDaemon) playAudio(filePath string) error {
	cmd := exec.Command("aplay", filePath)
	cmd.Stderr = nil
	return cmd.Run()
}

func (d *FixedTTSDaemon) processSpeak(sentences []string, speaker string) {
	if speaker == "" {
		speaker = defaultSpeaker
	}
	
	// Wait for audio system readiness
	<-d.playbackReady
	d.playbackReady <- true // Put it back
	
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	log.Printf("PROCESSING: %d sentences with throttling", len(sentences))
	
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
	
	// Launch TTS requests with throttling
	var wg sync.WaitGroup
	ttsStart := time.Now()
	
	for _, chunk := range chunks {
		wg.Add(1)
		go d.downloadTTS(chunk, speaker, &wg)
	}
	
	// Stream playback as chunks become ready
	go func() {
		for _, chunk := range chunks {
			<-chunk.Ready // Wait for this specific chunk
			
			if chunk.Error != nil {
				log.Printf("ERROR: Chunk %d failed: %v", chunk.Index, chunk.Error)
				continue
			}
			
			timeToReady := chunk.EndTime.Sub(ttsStart)
			log.Printf("PLAYING: Chunk %d ready in %v", chunk.Index, timeToReady)
			
			// Play immediately
			if err := d.playAudio(chunk.FilePath); err != nil {
				log.Printf("ERROR: Playback chunk %d: %v", chunk.Index, err)
			}
			
			// Clean up
			os.Remove(chunk.FilePath)
		}
	}()
	
	// Background completion tracking
	go func() {
		wg.Wait()
		log.Printf("COMPLETE: All %d chunks processed", len(sentences))
	}()
}

func (d *FixedTTSDaemon) handleSpeak(w http.ResponseWriter, r *http.Request) {
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
	
	// Process immediately in background
	go d.processSpeak(req.Sentences, req.Speaker)
	
	// Instant response
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("PROCESSING: %d sentences with throttling", len(req.Sentences)),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (d *FixedTTSDaemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":          "fixed-ready",
		"max_concurrent":  maxConcurrent,
		"audio_ready":     len(d.playbackReady) > 0,
		"throttle_slots":  cap(d.throttle) - len(d.throttle),
		"timestamp":       time.Now().Unix(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func main() {
	daemon := NewFixedTTSDaemon()
	
	http.HandleFunc("/speak", daemon.handleSpeak)
	http.HandleFunc("/health", daemon.handleHealth)
	
	server := &http.Server{
		Addr:         serverPort,
		Handler:      nil,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		log.Println("Shutting down fixed TTS daemon...")
		server.Close()
		os.RemoveAll(daemon.outputDir)
	}()
	
	log.Printf("FIXED TTS daemon ready on port %s (max concurrent: %d)", serverPort, maxConcurrent)
	
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}