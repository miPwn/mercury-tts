package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
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
	directTTSURL   = "http://localhost:5002/api/tts" // Direct localhost access
	defaultSpeaker = "p245" // Try p245 for better voice
	maxRetries     = 3
	serverPort     = ":8081"
)

type TTSRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
	UseDirect bool     `json:"use_direct,omitempty"` // Bypass LoadBalancer
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
	RouteUsed         string        `json:"route_used"`
}

type NetworkTrace struct {
	DNSLookup    time.Duration
	TCPConnect   time.Duration
	TLSHandshake time.Duration
	FirstByte    time.Duration
}

type TTSChunk struct {
	Index        int
	Text         string
	FilePath     string
	Ready        chan bool
	Error        error
	StartTime    time.Time
	EndTime      time.Time
	NetworkTrace *NetworkTrace
	RetryCount   int
}

type TTSDaemon struct {
	directClient   *http.Client
	lbClient       *http.Client
	outputDir      string
	mu             sync.RWMutex
	stats          map[string]*Performance
	connPool       *sync.Pool
}

func NewTTSDaemon() *TTSDaemon {
	outputDir := "/tmp/tts_daemon_cache"
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)

	// Optimized transport for direct cluster access
	directTransport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     300 * time.Second,
		DisableKeepAlives:   false,
		MaxConnsPerHost:     50,
		
		// TCP optimization
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second,  // Fast connection timeout
			KeepAlive: 120 * time.Second,
			DualStack: true,
		}).DialContext,
		
		// HTTP/2 disabled for simpler connection pooling
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		
		// Response header timeout
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		
		// Compression
		DisableCompression: false,
	}

	// LoadBalancer client (may have additional routing overhead)
	lbTransport := &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 25,
		IdleConnTimeout:     180 * time.Second,
		DisableKeepAlives:   false,
		MaxConnsPerHost:     25,
		
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,  // Longer timeout for LoadBalancer
			KeepAlive: 120 * time.Second,
			DualStack: true,
		}).DialContext,
		
		TLSNextProto:          make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		DisableCompression:    false,
	}

	daemon := &TTSDaemon{
		directClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: directTransport,
		},
		lbClient: &http.Client{
			Timeout:   45 * time.Second,
			Transport: lbTransport,
		},
		outputDir: outputDir,
		stats:     make(map[string]*Performance),
	}

	// Pre-warm both routes
	daemon.preWarmService()
	
	return daemon
}

func (d *TTSDaemon) preWarmService() {
	log.Println("Pre-warming TTS service on both routes...")
	
	var wg sync.WaitGroup
	
	// Warm direct route
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s?text=warmup_direct&speaker_id=%s", directTTSURL, defaultSpeaker), nil)
		resp, err := d.directClient.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
			log.Printf("Direct route warmed up in %v", time.Since(start))
		} else {
			log.Printf("Direct route warmup failed: %v", err)
		}
	}()
	
	// Warm LoadBalancer route
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s?text=warmup_lb&speaker_id=%s", ttsURL, defaultSpeaker), nil)
		resp, err := d.lbClient.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
			log.Printf("LoadBalancer route warmed up in %v", time.Since(start))
		} else {
			log.Printf("LoadBalancer route warmup failed: %v", err)
		}
	}()
	
	wg.Wait()
}

func (d *TTSDaemon) generateTTS(chunk *TTSChunk, wg *sync.WaitGroup, speaker string, useDirect bool) {
	defer wg.Done()
	
	chunk.StartTime = time.Now()
	encodedText := url.QueryEscape(chunk.Text)
	
	var requestURL string
	var client *http.Client
	var route string
	
	if useDirect {
		requestURL = fmt.Sprintf("%s?text=%s&speaker_id=%s", directTTSURL, encodedText, speaker)
		client = d.directClient
		route = "direct"
	} else {
		requestURL = fmt.Sprintf("%s?text=%s&speaker_id=%s", ttsURL, encodedText, speaker)
		client = d.lbClient
		route = "loadbalancer"
	}
	
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		chunk.RetryCount = attempt
		err = d.downloadAudioWithTrace(requestURL, chunk.FilePath, client, chunk)
		if err == nil {
			break
		}
		log.Printf("Chunk %d attempt %d failed (%s): %v", chunk.Index, attempt+1, route, err)
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}
	
	chunk.EndTime = time.Now()
	chunk.Error = err
	close(chunk.Ready)
}

func (d *TTSDaemon) downloadAudioWithTrace(requestURL, filePath string, client *http.Client, chunk *TTSChunk) error {
	trace := &NetworkTrace{}
	chunk.NetworkTrace = trace
	
	var dnsStart, connectStart, tlsStart, firstByteStart time.Time
	
	// Create HTTP trace to measure network performance
	clientTrace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			if !dnsStart.IsZero() {
				trace.DNSLookup = time.Since(dnsStart)
			}
		},
		ConnectStart: func(_, _ string) {
			connectStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			if !connectStart.IsZero() {
				trace.TCPConnect = time.Since(connectStart)
			}
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			if !tlsStart.IsZero() {
				trace.TLSHandshake = time.Since(tlsStart)
			}
		},
		GotFirstResponseByte: func() {
			if !firstByteStart.IsZero() {
				trace.FirstByte = time.Since(firstByteStart)
			}
		},
	}
	
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return err
	}
	
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), clientTrace))
	firstByteStart = time.Now()
	
	resp, err := client.Do(req)
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

func (d *TTSDaemon) streamTTS(sentences []string, speaker string, useDirect bool) (*TTSResponse, error) {
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
		go d.generateTTS(chunk, &wg, speaker, useDirect)
	}
	
	// Wait for first chunk to complete
	var firstChunkTime time.Time
	go func() {
		<-chunks[0].Ready
		firstChunkTime = time.Now()
	}()
	
	// Wait for all generation to complete
	wg.Wait()
	ttsEnd := time.Now()
	
	// Collect audio files and network metrics
	var audioFiles []string
	var errorCount, totalRetries, connectionsReused int
	var totalDNS, totalTCP, totalTLS, totalFirstByte time.Duration
	
	for _, chunk := range chunks {
		if chunk.Error != nil {
			errorCount++
			log.Printf("Chunk %d error: %v", chunk.Index, chunk.Error)
		} else {
			audioFiles = append(audioFiles, chunk.FilePath)
		}
		
		totalRetries += chunk.RetryCount
		
		if chunk.NetworkTrace != nil {
			totalDNS += chunk.NetworkTrace.DNSLookup
			totalTCP += chunk.NetworkTrace.TCPConnect
			totalTLS += chunk.NetworkTrace.TLSHandshake
			totalFirstByte += chunk.NetworkTrace.FirstByte
			
			// Connection was reused if DNS + TCP time is near zero
			if chunk.NetworkTrace.DNSLookup < time.Millisecond && chunk.NetworkTrace.TCPConnect < time.Millisecond {
				connectionsReused++
			}
		}
	}
	
	route := "loadbalancer"
	if useDirect {
		route = "direct"
	}
	
	// Calculate performance metrics
	perf := &Performance{
		TotalDuration:     time.Since(startTime),
		TTSGeneration:     ttsEnd.Sub(ttsStart),
		FirstChunkReady:   firstChunkTime.Sub(ttsStart),
		DNSLookup:         totalDNS / time.Duration(len(chunks)),
		TCPConnect:        totalTCP / time.Duration(len(chunks)),
		TLSHandshake:      totalTLS / time.Duration(len(chunks)),
		ServerProcessing:  (ttsEnd.Sub(ttsStart) - totalFirstByte) / time.Duration(len(chunks)),
		ContentTransfer:   totalFirstByte / time.Duration(len(chunks)),
		ChunksGenerated:   len(audioFiles),
		ConcurrentJobs:    len(sentences),
		ConnectionsReused: connectionsReused,
		RetryAttempts:     totalRetries,
		RouteUsed:         route,
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
	
	route := "LoadBalancer"
	if req.UseDirect {
		route = "Direct"
	}
	
	log.Printf("Processing TTS request: %d sentences, speaker: %s, route: %s", 
		len(req.Sentences), req.Speaker, route)
	
	response, err := d.streamTTS(req.Sentences, req.Speaker, req.UseDirect)
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
	
	if !filepath.HasPrefix(audioFile, d.outputDir) {
		http.Error(w, "Invalid file path", http.StatusForbidden)
		return
	}
	
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

func (d *TTSDaemon) handleBenchmark(w http.ResponseWriter, r *http.Request) {
	// Run comparative benchmark between direct and LoadBalancer routes
	testSentences := []string{
		"This is a network performance test.",
		"Measuring LoadBalancer versus direct routing.",
		"Analyzing transport layer optimization.",
	}
	
	log.Println("Running comparative benchmark...")
	
	// Test LoadBalancer route
	lbResponse, _ := d.streamTTS(testSentences, defaultSpeaker, false)
	
	// Test Direct route
	directResponse, _ := d.streamTTS(testSentences, defaultSpeaker, true)
	
	benchmark := map[string]interface{}{
		"loadbalancer": lbResponse.Performance,
		"direct":       directResponse.Performance,
		"timestamp":    time.Now().Unix(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(benchmark)
}

func (d *TTSDaemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Test both routes
	var wg sync.WaitGroup
	results := make(map[string]interface{})
	
	wg.Add(2)
	
	// Test direct route
	go func() {
		defer wg.Done()
		start := time.Now()
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s?text=health&speaker_id=%s", directTTSURL, defaultSpeaker), nil)
		resp, err := d.directClient.Do(req)
		
		if err == nil && resp != nil {
			resp.Body.Close()
			results["direct"] = map[string]interface{}{
				"status":   "healthy",
				"latency":  time.Since(start).Milliseconds(),
			}
		} else {
			results["direct"] = map[string]interface{}{
				"status": "error",
				"error":  err.Error(),
			}
		}
	}()
	
	// Test LoadBalancer route
	go func() {
		defer wg.Done()
		start := time.Now()
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s?text=health&speaker_id=%s", ttsURL, defaultSpeaker), nil)
		resp, err := d.lbClient.Do(req)
		
		if err == nil && resp != nil {
			resp.Body.Close()
			results["loadbalancer"] = map[string]interface{}{
				"status":   "healthy",
				"latency":  time.Since(start).Milliseconds(),
			}
		} else {
			results["loadbalancer"] = map[string]interface{}{
				"status": "error",
				"error":  err.Error(),
			}
		}
	}()
	
	wg.Wait()
	
	results["daemon"] = "healthy"
	results["timestamp"] = time.Now().Unix()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func main() {
	daemon := NewTTSDaemon()
	
	// Setup HTTP routes
	http.HandleFunc("/tts", daemon.handleTTSRequest)
	http.HandleFunc("/play", daemon.handlePlayAudio)
	http.HandleFunc("/stats", daemon.handleStats)
	http.HandleFunc("/health", daemon.handleHealth)
	http.HandleFunc("/benchmark", daemon.handleBenchmark)
	
	// Graceful shutdown
	server := &http.Server{
		Addr:    serverPort,
		Handler: nil,
	}
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		log.Println("Shutting down optimized TTS daemon...")
		
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		server.Shutdown(ctx)
		os.RemoveAll(daemon.outputDir)
	}()
	
	log.Printf("Optimized TTS daemon starting on port %s", serverPort)
	log.Printf("Endpoints: /tts, /play, /stats, /health, /benchmark")
	log.Printf("Routes: LoadBalancer (%s) and Direct (%s)", ttsURL, directTTSURL)
	
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
	
	log.Println("Optimized TTS daemon shut down gracefully")
}