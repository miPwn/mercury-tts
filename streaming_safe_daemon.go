package main

import (
	"bytes"
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
	// Use load balancer service for automatic pod discovery
	ttsURL1        = "http://localhost:5002/api/tts" // Load balancer service
	ttsURL2        = "http://localhost:5002/api/tts" // Same service for reliability
	defaultSpeaker = "p245"
	dragThreshold  = 3 * time.Second
	defaultBind    = "0.0.0.0:8091" // Default IPv4 bind; can be overridden via env TTS_BIND_ADDR
	warmupCount    = 5
	maxRetries     = 2
	// Latency optimization: multiple TTS endpoints for load distribution
	maxConcurrentGeneration = 10 // Aggressive concurrent generation
	// Zero-latency logging
	logBufferSize = 1000 // Async log buffer size
	logFile       = "/var/log/tts-daemon.log"
)

type SpeakRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
}

type TTSChunk struct {
	Index         int
	Text          string
	FilePath      string
	Ready         chan bool
	StartTime     time.Time
	EndTime       time.Time
	Error         error
	RetryCount    int
	PlayCompleted chan bool // Signals when playback is done
	TTSEndpoint   string    // Load balanced TTS endpoint
	Predecessor   *TTSChunk // Link to previous chunk for ordering
}

// AsyncLogger provides zero-latency logging
type AsyncLogger struct {
	logChan chan string
	logFile *os.File
	done    chan bool
}

// NewAsyncLogger creates a zero-latency async logger
func NewAsyncLogger(filename string) (*AsyncLogger, error) {
	logFile, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// Fallback to /tmp if /var/log not writable
		tmpLogFile := "/tmp/tts-daemon.log"
		logFile, err = os.OpenFile(tmpLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("unable to create log file: %v", err)
		}
	}
	
	logger := &AsyncLogger{
		logChan: make(chan string, logBufferSize),
		logFile: logFile,
		done:    make(chan bool),
	}
	
	// Start async log writer goroutine
	go logger.writer()
	
	return logger, nil
}

// Log sends message to async buffer (zero-latency)
func (al *AsyncLogger) Log(format string, args ...interface{}) {
	timestamp := time.Now().Format("2006/01/02 15:04:05.000")
	message := fmt.Sprintf("[%s] %s\n", timestamp, fmt.Sprintf(format, args...))
	
	// Non-blocking send to buffer
	select {
	case al.logChan <- message:
		// Successfully queued
	default:
		// Buffer full - drop message to maintain zero latency
		// This ensures logging never blocks the audio pipeline
	}
}

// writer handles async log writing (runs in separate goroutine)
func (al *AsyncLogger) writer() {
	for {
		select {
		case message := <-al.logChan:
			al.logFile.WriteString(message)
			al.logFile.Sync() // Ensure immediate write to disk
		case <-al.done:
			// Drain remaining messages
			for len(al.logChan) > 0 {
				message := <-al.logChan
				al.logFile.WriteString(message)
			}
			al.logFile.Close()
			return
		}
	}
}

// Close gracefully shuts down async logger
func (al *AsyncLogger) Close() {
	close(al.done)
}

func NewStreamingDaemon() *StreamingDaemon {
	outputDir := "/tmp/streaming_safe_daemon"
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)

	// Initialize zero-latency async logger
	logger, err := NewAsyncLogger(logFile)
	if err != nil {
		log.Printf("Warning: Could not initialize async logger: %v", err)
		logger = nil // Continue without file logging
	}

	warmupPool := make(chan *http.Client, warmupCount)
	
	daemon := &StreamingDaemon{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   20,
				IdleConnTimeout:       300 * time.Second,
				DisableKeepAlives:     false,
				ResponseHeaderTimeout: 10 * time.Second,
			},
		},
		outputDir:     outputDir,
		playbackReady: make(chan bool, 1),
		warmupPool:    warmupPool,
		// Initialize load-balanced TTS endpoints (only working pods)
		ttsEndpoints:  []string{ttsURL1, ttsURL2},
		playbackChain: make(chan *TTSChunk, maxConcurrentGeneration),
		logger:        logger,
	}

	if daemon.logger != nil {
		daemon.logger.Log("INIT: Streaming daemon initializing with async logging")
	}

	daemon.preWarmSystem()
	return daemon
}

type StreamingDaemon struct {
	client        *http.Client
	outputDir     string
	playbackReady chan bool
	warmupPool    chan *http.Client
	// Removed audioMutex - using intelligent coordination instead
	ttsEndpoints  []string  // Multiple TTS endpoints for load balancing
	playbackChain chan *TTSChunk // Sequential playback coordination
	logger        *AsyncLogger // Zero-latency logging
}


func (d *StreamingDaemon) preWarmSystem() {
	log.Println("Pre-warming streaming system...")
	
	var wg sync.WaitGroup
	
	// Pre-warm HTTP connections
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
					ResponseHeaderTimeout: 10 * time.Second,
				},
			}
			
			req, _ := http.NewRequest("GET", fmt.Sprintf("%s?text=warmup%d&speaker_id=%s", ttsURL1, i, defaultSpeaker), nil)
			resp, err := client.Do(req)
			if err == nil && resp != nil {
				resp.Body.Close()
				d.warmupPool <- client
				log.Printf("Pre-warmed connection %d", i+1)
			}
		}
	}()
	
	// Pre-warm audio system
	wg.Add(1)
	go func() {
		defer wg.Done()
		silentFile := filepath.Join(d.outputDir, "silent.wav")
		
		resp, err := d.client.Get(fmt.Sprintf("%s?text=.&speaker_id=%s", ttsURL1, defaultSpeaker))
		if err == nil && resp != nil {
			defer resp.Body.Close()
			if file, err := os.Create(silentFile); err == nil {
				io.Copy(file, resp.Body)
				file.Close()
				
				cmd := exec.Command("aplay", "-D", "default", silentFile)
				cmd.Run()
				os.Remove(silentFile)
				
				log.Println("Audio system pre-warmed")
				d.playbackReady <- true
			}
		}
	}()
	
	wg.Wait()
	log.Printf("Streaming system ready: %d connections warmed", len(d.warmupPool))
}

func (d *StreamingDaemon) getWarmedClient() *http.Client {
	select {
	case client := <-d.warmupPool:
		return client
	default:
		return d.client
	}
}

func (d *StreamingDaemon) returnWarmedClient(client *http.Client) {
	select {
	case d.warmupPool <- client:
	default:
	}
}

func (d *StreamingDaemon) downloadTTSWithRetry(chunk *TTSChunk, speaker string, wg *sync.WaitGroup) {
	defer wg.Done()
	
	// Load balance across available TTS endpoints for maximum parallelization
	endpointIndex := chunk.Index % len(d.ttsEndpoints)
	chunk.TTSEndpoint = d.ttsEndpoints[endpointIndex]
	
	for attempt := 0; attempt <= maxRetries; attempt++ {
		chunk.RetryCount = attempt
		
		client := d.getWarmedClient()
		chunk.StartTime = time.Now()
		encodedText := url.QueryEscape(chunk.Text)
		requestURL := fmt.Sprintf("%s?text=%s&speaker_id=%s", chunk.TTSEndpoint, encodedText, speaker)
		
		resp, err := client.Get(requestURL)
		d.returnWarmedClient(client)
		
		if err != nil {
			if attempt < maxRetries {
				log.Printf("RETRY: Chunk %d attempt %d failed: %v", chunk.Index, attempt+1, err)
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond) // Exponential backoff
				continue
			}
			chunk.Error = err
			close(chunk.Ready)
			return
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			if attempt < maxRetries {
				log.Printf("RETRY: Chunk %d HTTP %d, attempt %d", chunk.Index, resp.StatusCode, attempt+1)
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
				continue
			}
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
			log.Printf("DRAG: Chunk %d took %v (attempt %d)", chunk.Index, duration, attempt+1)
		}
		
		if err == nil {
			break // Success
		}
		
		if attempt < maxRetries {
			log.Printf("RETRY: Chunk %d file error: %v, attempt %d", chunk.Index, err, attempt+1)
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
		}
	}
	
	close(chunk.Ready)
}

func (d *StreamingDaemon) playAudioOptimized(chunk *TTSChunk) {
	// Wait for predecessor to complete if it exists
	if chunk.Predecessor != nil && chunk.Predecessor.PlayCompleted != nil {
		<-chunk.Predecessor.PlayCompleted
	}
	
	log.Printf("PLAY: Immediate playback chunk %d (latency optimized)", chunk.Index)
	
	cmd := exec.Command("aplay", "-D", "default", chunk.FilePath)
	cmd.Stderr = nil
	
	if err := cmd.Run(); err != nil {
		log.Printf("ERROR: Playback chunk %d failed: %v", chunk.Index, err)
	}
	
	// Signal playback completion immediately
	close(chunk.PlayCompleted)
	
	// Aggressive cleanup for minimum resource usage
	go func() {
		time.Sleep(50 * time.Millisecond) // Reduced delay for faster cleanup
		os.Remove(chunk.FilePath)
	}()
}

func (d *StreamingDaemon) streamingPlaybackOptimized(chunks []*TTSChunk, ttsStart time.Time) {
	// LATENCY OPTIMIZATION: Setup predecessor chain for sequential coordination
	for i := range chunks {
		if i > 0 {
			chunks[i].Predecessor = chunks[i-1]
		}
		chunks[i].PlayCompleted = make(chan bool)
	}
	
	// SEQUENTIAL PLAYBACK COORDINATION: Process chunks in strict order
	go func() {
		for i := range chunks {
			chunk := chunks[i]
			
			// Wait for TTS generation to complete for this chunk
			<-chunk.Ready
			
			if chunk.Error != nil {
				log.Printf("ERROR: Chunk %d failed: %v", chunk.Index, chunk.Error)
				// Signal completion even for errors to unblock chain
				close(chunk.PlayCompleted)
				continue
			}
			
			timeToReady := chunk.EndTime.Sub(ttsStart)
			log.Printf("READY: Chunk %d ready in %v (endpoint: %s)", chunk.Index, timeToReady, chunk.TTSEndpoint)
			
			// Zero-latency async logging of chunk completion
			if daemon := d; daemon != nil && daemon.logger != nil {
				daemon.logger.Log("CHUNK: Ready chunk %d in %v via %s", chunk.Index, timeToReady, chunk.TTSEndpoint)
			}
			
			// SEQUENTIAL PLAYBACK: Wait for predecessor to complete before starting
			if chunk.Predecessor != nil && chunk.Predecessor.PlayCompleted != nil {
				<-chunk.Predecessor.PlayCompleted
			}
			
			log.Printf("PLAY: Sequential playback chunk %d (order guaranteed)", chunk.Index)
			
			// Validate file existence and permissions
			if fileInfo, err := os.Stat(chunk.FilePath); err != nil {
				log.Printf("ERROR: Chunk %d file not accessible: %v", chunk.Index, err)
				close(chunk.PlayCompleted)
				continue
			} else {
				log.Printf("FILE: Chunk %d validated - size: %d bytes, path: %s", chunk.Index, fileInfo.Size(), chunk.FilePath)
			}
			
			// Execute audio playback synchronously to maintain order
			log.Printf("AUDIO: Starting playback chunk %d - file: %s", chunk.Index, chunk.FilePath)
			cmd := exec.Command("aplay", "-D", "default", chunk.FilePath)
			
			// Capture both stdout and stderr for detailed logging
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			
			playbackStart := time.Now()
			if err := cmd.Run(); err != nil {
				log.Printf("ERROR: Playback chunk %d failed: %v | stderr: %s | stdout: %s", chunk.Index, err, stderr.String(), stdout.String())
			} else {
				playbackDuration := time.Since(playbackStart)
				log.Printf("SUCCESS: Playback chunk %d completed in %v | stderr: %s", chunk.Index, playbackDuration, stderr.String())
			}
			
			// Signal playback completion immediately
			close(chunk.PlayCompleted)
			
			// Aggressive cleanup for minimum resource usage
			go func(filePath string) {
				time.Sleep(50 * time.Millisecond)
				os.Remove(filePath)
			}(chunk.FilePath)
		}
	}()
	
	log.Printf("LATENCY-OPTIMIZED: All %d chunks processing concurrently with SEQUENTIAL coordination", len(chunks))
}

func (d *StreamingDaemon) processSpeak(sentences []string, speaker string) {
	if speaker == "" {
		speaker = defaultSpeaker
	}

	// Wait for audio system readiness
	<-d.playbackReady
	d.playbackReady <- true

	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	log.Printf("LATENCY-OPTIMIZED: Processing %d sentences with concurrent generation", len(sentences))
	
	// Zero-latency async logging
	if d.logger != nil {
		d.logger.Log("REQUEST: Processing %d sentences, speaker=%s, timestamp=%s", len(sentences), speaker, timestamp)
	}
	
	// Create chunks with load balancing preparation
	chunks := make([]*TTSChunk, len(sentences))
	for i, sentence := range sentences {
		chunks[i] = &TTSChunk{
			Index:    i + 1,
			Text:     sentence,
			FilePath: filepath.Join(d.outputDir, fmt.Sprintf("%s_%d.wav", timestamp, i+1)),
			Ready:    make(chan bool),
		}
	}
	
	// CONCURRENT GENERATION: Launch all TTS requests immediately across load-balanced endpoints
	var wg sync.WaitGroup
	ttsStart := time.Now()
	
	for _, chunk := range chunks {
		wg.Add(1)
		go d.downloadTTSWithRetry(chunk, speaker, &wg)
	}
	
	// Start optimized streaming playback with predecessor coordination
	go d.streamingPlaybackOptimized(chunks, ttsStart)
	
	// Background completion tracking
	go func() {
		wg.Wait()
		log.Printf("COMPLETE: All TTS requests finished (latency optimized)")
	}()
}

func (d *StreamingDaemon) handleSpeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SpeakRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if d.logger != nil {
			d.logger.Log("ERROR: Invalid JSON from %s: %v", r.RemoteAddr, err)
		}
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(req.Sentences) == 0 {
		if d.logger != nil {
			d.logger.Log("ERROR: Empty sentences from %s", r.RemoteAddr)
		}
		http.Error(w, "No sentences provided", http.StatusBadRequest)
		return
	}
	
	// Log incoming request
	if d.logger != nil {
		d.logger.Log("API: Incoming request from %s - %d sentences, speaker=%s", r.RemoteAddr, len(req.Sentences), req.Speaker)
	}
	
	// Process with streaming playback
	go d.processSpeak(req.Sentences, req.Speaker)
	
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("STREAMING: Processing %d sentences safely", len(req.Sentences)),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (d *StreamingDaemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":         "streaming-safe-ready",
		"warmed_clients": len(d.warmupPool),
		"audio_ready":    len(d.playbackReady) > 0,
		"timestamp":      time.Now().Unix(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleSpeakClient generates audio files and returns downloadable URLs for client-side playback.
func (d *StreamingDaemon) handleSpeakClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SpeakRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if d.logger != nil {
			d.logger.Log("ERROR: Invalid JSON (client) from %s: %v", r.RemoteAddr, err)
		}
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if len(req.Sentences) == 0 {
		http.Error(w, "No sentences provided", http.StatusBadRequest)
		return
	}

	if req.Speaker == "" {
		req.Speaker = defaultSpeaker
	}

	d.generateAndRespond(w, req.Sentences, req.Speaker)
}

// GET variant to avoid JSON body issues: /speak_client_get?q=...&q=...&speaker=p254
func (d *StreamingDaemon) handleSpeakClientGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()["q"]
	if len(q) == 0 {
		http.Error(w, "No sentences provided", http.StatusBadRequest)
		return
	}
	speaker := r.URL.Query().Get("speaker")
	if speaker == "" {
		speaker = defaultSpeaker
	}
	d.generateAndRespond(w, q, speaker)
}

// Shared generator that produces WAV files and responds with URLs.
func (d *StreamingDaemon) generateAndRespond(w http.ResponseWriter, sentences []string, speaker string) {
	// Unique prefix to avoid collisions; reuse timestamp like /speak path.
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	chunks := make([]*TTSChunk, len(sentences))
	for i, sentence := range sentences {
		chunks[i] = &TTSChunk{
			Index:    i + 1,
			Text:     sentence,
			FilePath: filepath.Join(d.outputDir, fmt.Sprintf("%s_%d.wav", timestamp, i+1)),
			Ready:    make(chan bool),
		}
	}
	var wg sync.WaitGroup
	for _, chunk := range chunks {
		wg.Add(1)
		go d.downloadTTSWithRetry(chunk, speaker, &wg)
	}
	wg.Wait()
	urls := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.Error != nil {
			if d.logger != nil {
				d.logger.Log("ERROR: Client chunk %d failed: %v", chunk.Index, chunk.Error)
			}
			continue
		}
		urls = append(urls, fmt.Sprintf("/audio/%s_%d.wav", timestamp, chunk.Index))
	}
	resp := map[string]interface{}{
		"success":    len(urls) > 0,
		"audio_urls": urls,
		"speaker":    speaker,
		"message":    fmt.Sprintf("READY: %d/%d chunks generated", len(urls), len(chunks)),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func getBindAddr() string {
	if v := os.Getenv("TTS_BIND_ADDR"); v != "" {
		return v
	}
	return defaultBind
}

func main() {
	daemon := NewStreamingDaemon()
	
	http.HandleFunc("/speak", daemon.handleSpeak)
	http.HandleFunc("/health", daemon.handleHealth)
	// Serve generated WAV files for client playback
	http.Handle("/audio/", http.StripPrefix("/audio/", http.FileServer(http.Dir(daemon.outputDir))))
	// Client-facing endpoints that return downloadable URLs rather than playing locally
	http.HandleFunc("/speak_client", daemon.handleSpeakClient)
	http.HandleFunc("/speak_client_get", daemon.handleSpeakClientGet)
	
	server := &http.Server{
		Addr:         getBindAddr(), // Bind address is configurable via env TTS_BIND_ADDR
		Handler:      nil,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 3 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		log.Println("Shutting down streaming safe daemon...")
		server.Close()
		os.RemoveAll(daemon.outputDir)
	}()
	
	bindAddr := getBindAddr()
	log.Printf("STREAMING SAFE TTS daemon ready on %s (localhost access at http://localhost:8091)", bindAddr)
	
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
