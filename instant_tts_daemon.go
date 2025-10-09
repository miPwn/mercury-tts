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
	warmupCount    = 5 // Pre-warm connections
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

type InstantTTSDaemon struct {
	client        *http.Client
	outputDir     string
	playbackReady chan bool
	warmupPool    chan *http.Client // Pool of pre-warmed clients
}

func NewInstantTTSDaemon() *InstantTTSDaemon {
	outputDir := "/tmp/instant_tts_daemon"
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)

	// Create multiple pre-warmed HTTP clients
	warmupPool := make(chan *http.Client, warmupCount)
	
	daemon := &InstantTTSDaemon{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   20,
				IdleConnTimeout:       300 * time.Second,
				DisableKeepAlives:     false,
				ResponseHeaderTimeout: 5 * time.Second,
			},
		},
		outputDir:     outputDir,
		playbackReady: make(chan bool, 1),
		warmupPool:    warmupPool,
	}

	// Pre-warm connections and audio system
	daemon.preWarmSystem()
	
	return daemon
}

func (d *InstantTTSDaemon) preWarmSystem() {
	log.Println("Pre-warming system for instant response...")
	
	var wg sync.WaitGroup
	
	// 1. Pre-warm HTTP connections
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < warmupCount; i++ {
			client := &http.Client{
				Timeout: 30 * time.Second,
				Transport: &http.Transport{
					MaxIdleConns:          50,
					MaxIdleConnsPerHost:   10,
					IdleConnTimeout:       300 * time.Second,
					DisableKeepAlives:     false,
					ResponseHeaderTimeout: 5 * time.Second,
				},
			}
			
			// Make a dummy request to establish connection
			req, _ := http.NewRequest("GET", fmt.Sprintf("%s?text=warmup%d&speaker_id=%s", ttsURL, i, defaultSpeaker), nil)
			resp, err := client.Do(req)
			if err == nil && resp != nil {
				resp.Body.Close()
				d.warmupPool <- client
				log.Printf("Pre-warmed connection %d", i+1)
			}
		}
	}()
	
	// 2. Pre-warm audio system
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Create a tiny silent audio file to initialize aplay
		silentFile := filepath.Join(d.outputDir, "silent.wav")
		
		// Generate 100ms of silence
		resp, err := d.client.Get(fmt.Sprintf("%s?text=.&speaker_id=%s", ttsURL, defaultSpeaker))
		if err == nil && resp != nil {
			defer resp.Body.Close()
			if file, err := os.Create(silentFile); err == nil {
				io.Copy(file, resp.Body)
				file.Close()
				
				// Play silent audio to warm up audio system
				cmd := exec.Command("aplay", silentFile)
				cmd.Run()
				os.Remove(silentFile)
				
				log.Println("Audio system pre-warmed")
				d.playbackReady <- true
			}
		}
	}()
	
	wg.Wait()
	log.Printf("System pre-warmed: %d connections ready, audio system initialized", len(d.warmupPool))
}

func (d *InstantTTSDaemon) getWarmedClient() *http.Client {
	select {
	case client := <-d.warmupPool:
		return client
	default:
		// Fallback to default client if pool is empty
		return d.client
	}
}

func (d *InstantTTSDaemon) returnWarmedClient(client *http.Client) {
	select {
	case d.warmupPool <- client:
		// Returned to pool
	default:
		// Pool is full, discard
	}
}

func (d *InstantTTSDaemon) downloadTTS(chunk *TTSChunk, speaker string, wg *sync.WaitGroup) {
	defer wg.Done()
	
	client := d.getWarmedClient()
	defer d.returnWarmedClient(client)
	
	chunk.StartTime = time.Now()
	encodedText := url.QueryEscape(chunk.Text)
	requestURL := fmt.Sprintf("%s?text=%s&speaker_id=%s", ttsURL, encodedText, speaker)
	
	resp, err := client.Get(requestURL)
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
	
	duration := chunk.EndTime.Sub(chunk.StartTime)
	if duration > dragThreshold {
		log.Printf("DRAG: Chunk %d took %v", chunk.Index, duration)
	}
	
	close(chunk.Ready)
}

func (d *InstantTTSDaemon) playAudio(filePath string) error {
	// Use pre-warmed audio system
	cmd := exec.Command("aplay", filePath)
	cmd.Stderr = nil
	return cmd.Run()
}

func (d *InstantTTSDaemon) processSpeak(sentences []string, speaker string) {
	if speaker == "" {
		speaker = defaultSpeaker
	}
	
	// Wait for audio system to be ready (should be instant after warmup)
	<-d.playbackReady
	d.playbackReady <- true // Put it back for next use
	
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	log.Printf("INSTANT: Processing %d sentences", len(sentences))
	
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
	
	// Launch all TTS requests IMMEDIATELY with pre-warmed connections
	var wg sync.WaitGroup
	ttsStart := time.Now()
	
	for _, chunk := range chunks {
		wg.Add(1)
		go d.downloadTTS(chunk, speaker, &wg)
	}
	
	// Stream playback as chunks become ready - NO WAITING
	go func() {
		for _, chunk := range chunks {
			<-chunk.Ready // This blocks until THIS chunk is ready
			
			if chunk.Error != nil {
				log.Printf("ERROR: Chunk %d failed: %v", chunk.Index, chunk.Error)
				continue
			}
			
			timeToReady := chunk.EndTime.Sub(ttsStart)
			log.Printf("PLAY: Chunk %d ready in %v", chunk.Index, timeToReady)
			
			// INSTANT playback with pre-warmed audio
			if err := d.playAudio(chunk.FilePath); err != nil {
				log.Printf("ERROR: Playback chunk %d: %v", chunk.Index, err)
			}
			
			// Clean up immediately
			os.Remove(chunk.FilePath)
		}
	}()
	
	// Background completion tracking
	go func() {
		wg.Wait()
		log.Printf("COMPLETE: All chunks processed")
	}()
}

func (d *InstantTTSDaemon) handleSpeak(w http.ResponseWriter, r *http.Request) {
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
	
	// INSTANT processing - no delays
	go d.processSpeak(req.Sentences, req.Speaker)
	
	// INSTANT response
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("INSTANT: Processing %d sentences", len(req.Sentences)),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (d *InstantTTSDaemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":           "instant-ready",
		"warmed_clients":   len(d.warmupPool),
		"audio_ready":      len(d.playbackReady) > 0,
		"timestamp":        time.Now().Unix(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func main() {
	daemon := NewInstantTTSDaemon()
	
	http.HandleFunc("/speak", daemon.handleSpeak)
	http.HandleFunc("/health", daemon.handleHealth)
	
	server := &http.Server{
		Addr:           serverPort,
		Handler:        nil,
		ReadTimeout:    1 * time.Second,  // Fast request handling
		WriteTimeout:   2 * time.Second,  // Fast response
		IdleTimeout:    60 * time.Second,
	}
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		log.Println("Shutting down instant TTS daemon...")
		server.Close()
		os.RemoveAll(daemon.outputDir)
	}()
	
	log.Printf("INSTANT TTS daemon ready on port %s", serverPort)
	
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}