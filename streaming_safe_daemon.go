//go:build tts_experimental
// +build tts_experimental

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"tts-pipeline-test/internal/observability"
)

const (
	defaultBind    = "0.0.0.0:8091" // Default IPv4 bind; can be overridden via env TTS_BIND_ADDR
)

var (
	defaultSpeaker = firstNonEmptyEnv("HAL_SPEAKER", "DEFAULT_SPEAKER", "p245")
	dragThreshold  = getEnvDuration("DRAG_THRESHOLD_SECONDS", 3*time.Second)
	warmupCount    = getEnvInt("WARMUP_CONNECTION_COUNT", 5)
	maxRetries     = getEnvInt("MAX_RETRIES", 2)
	// Latency optimization: multiple TTS endpoints for load distribution
	maxConcurrentGeneration = getEnvInt("MAX_CONCURRENT_GENERATION", 10)
	ttsURL1                = getEnvDefault("TTS_LOADBALANCER_URL", "http://127.0.0.1:5002/api/tts")
	ttsURL2                = getEnvDefault("TTS_DIRECT_URL", ttsURL1)
	// VBAN configuration
	playbackMode      = getEnvDefault("PLAYBACK_MODE", "aplay") // "aplay" or "vban"
	playbackDevice    = getEnvDefault("PLAYBACK_DEVICE", "default")
	vbanTargetIP      = getEnvDefault("VBAN_TARGET_IP", "192.168.1.100")
	vbanTargetPort    = getEnvDefault("VBAN_TARGET_PORT", "6980")
	vbanStreamName    = getEnvDefault("VBAN_STREAM_NAME", "Falcon")
	dotmatrixEnabled  = getEnvBool("HAL_DOTMATRIX_ENABLED", false)
	dotmatrixQueueDir = getEnvDefault("HAL_DOTMATRIX_QUEUE_DIR", "/tmp/halo-dotmatrix/queue")
	dotmatrixWavDir   = getEnvDefault("HAL_DOTMATRIX_WAV_DIR", "/tmp/halo-dotmatrix/wav")
	outputDirRoot     = getEnvDefault("TTS_TMP_AUDIO_DIR", "/tmp/streaming_safe_daemon")
	httpTimeout       = getEnvDuration("HTTP_TIMEOUT_SECONDS", 30*time.Second)
	httpHeaderTimeout = getEnvDuration("HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS", 10*time.Second)
	maxIdleConns      = getEnvInt("MAX_IDLE_CONNECTIONS", 100)
	maxIdleConnsHost  = getEnvInt("MAX_IDLE_CONNECTIONS_PER_HOST", 20)
	idleConnTimeout   = getEnvDuration("IDLE_CONNECTION_TIMEOUT_SECONDS", 300*time.Second)
	readTimeout       = getEnvDuration("DAEMON_READ_TIMEOUT", 2*time.Second)
	writeTimeout      = getEnvDuration("DAEMON_WRITE_TIMEOUT", 30*time.Second)
	idleTimeout       = getEnvDuration("DAEMON_IDLE_TIMEOUT", 60*time.Second)
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

func NewStreamingDaemon() *StreamingDaemon {
	outputDir := outputDirRoot
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)

	warmupPool := make(chan *http.Client, warmupCount)

	daemon := &StreamingDaemon{
		client: &http.Client{
			Timeout: httpTimeout,
			Transport: &http.Transport{
				MaxIdleConns:          maxIdleConns,
				MaxIdleConnsPerHost:   maxIdleConnsHost,
				IdleConnTimeout:       idleConnTimeout,
				DisableKeepAlives:     false,
				ResponseHeaderTimeout: httpHeaderTimeout,
			},
		},
		outputDir:     outputDir,
		playbackReady: make(chan bool, 1),
		warmupPool:    warmupPool,
		// Initialize load-balanced TTS endpoints (only working pods)
		ttsEndpoints:  []string{ttsURL1, ttsURL2},
		playbackChain: make(chan *TTSChunk, maxConcurrentGeneration),
	}

	select {
	case daemon.playbackReady <- true:
	default:
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
	ttsEndpoints  []string       // Multiple TTS endpoints for load balancing
	playbackChain chan *TTSChunk // Sequential playback coordination
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
				Timeout: httpTimeout,
				Transport: &http.Transport{
					MaxIdleConns:          maxIdleConns,
					MaxIdleConnsPerHost:   maxIdleConnsHost,
					IdleConnTimeout:       idleConnTimeout,
					DisableKeepAlives:     false,
					ResponseHeaderTimeout: httpHeaderTimeout,
				},
			}

			// Coqui TTS GET API warmup
			url := fmt.Sprintf("%s?text=warmup%d&speaker_id=%s", ttsURL1, i, defaultSpeaker)
			resp, err := client.Get(url)
			if err == nil && resp != nil {
				resp.Body.Close()
				d.warmupPool <- client
				log.Printf("Pre-warmed connection %d", i+1)
			}
		}
	}()

	// Pre-warm audio system in the background so startup never blocks on audio/VBAN
	go func() {
		defer func() {
			select {
			case d.playbackReady <- true:
			default:
			}
		}()

		silentFile := filepath.Join(d.outputDir, "silent.wav")
		defer os.Remove(silentFile)

		// Coqui TTS GET API for silent warmup
		url := fmt.Sprintf("%s?text=.&speaker_id=%s", ttsURL1, defaultSpeaker)
		resp, err := d.client.Get(url)
		if err != nil || resp == nil {
			log.Printf("WARN: Audio warmup TTS request failed: %v", err)
			return
		}
		defer resp.Body.Close()

		file, err := os.Create(silentFile)
		if err != nil {
			log.Printf("WARN: Could not create audio warmup file: %v", err)
			return
		}

		if _, err := io.Copy(file, resp.Body); err != nil {
			file.Close()
			log.Printf("WARN: Could not write audio warmup file: %v", err)
			return
		}
		file.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		var cmd *exec.Cmd
		if playbackMode == "vban" {
			cmd = exec.CommandContext(ctx, "sh", "-c",
				fmt.Sprintf("tail -c +45 %q | /usr/local/bin/vban_emitter -i %q -p %q -s %q -b pipe -f 16I -r 22050 -n 1",
					silentFile, vbanTargetIP, vbanTargetPort, vbanStreamName))
		} else {
			cmd = exec.CommandContext(ctx, "aplay", "-D", playbackDevice, silentFile)
		}

		if output, err := cmd.CombinedOutput(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				log.Printf("WARN: Audio system pre-warm timed out after 2s (mode: %s); continuing startup", playbackMode)
				return
			}
			log.Printf("WARN: Audio system pre-warm failed (mode: %s): %v | output: %s", playbackMode, err, string(output))
			return
		}

		log.Printf("Audio system pre-warmed (mode: %s)", playbackMode)
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

		// Coqui TTS GET API request
		url := fmt.Sprintf("%s?text=%s&speaker_id=%s", chunk.TTSEndpoint, neturl.QueryEscape(chunk.Text), speaker)

		resp, err := client.Get(url)
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

	cmd := exec.Command("aplay", "-D", playbackDevice, chunk.FilePath)
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

func streamPCMToFIFORealTime(wavPath, fifoPath string, holdOpen time.Duration) error {
const (
pcmOffset      = 44
packetBytes    = 512
sampleRate     = 22050
channels       = 1
bytesPerSample = 2
)

fifoFile, err := os.OpenFile(fifoPath, os.O_WRONLY, 0600)
if err != nil {
return err
}
defer fifoFile.Close()

wavFile, err := os.Open(wavPath)
if err != nil {
return err
}
defer wavFile.Close()

if _, err := wavFile.Seek(pcmOffset, io.SeekStart); err != nil {
return err
}

packetDuration := time.Second * time.Duration(packetBytes) / time.Duration(sampleRate*channels*bytesPerSample)
buf := make([]byte, packetBytes)
nextWrite := time.Now()
streamStart := nextWrite

for {
n, err := io.ReadFull(wavFile, buf)
lastChunk := false
if err == io.EOF {
break
}
if err == io.ErrUnexpectedEOF {
for i := n; i < packetBytes; i++ {
buf[i] = 0
}
lastChunk = true
} else if err != nil {
return err
}

if _, err := fifoFile.Write(buf); err != nil {
return err
}

nextWrite = nextWrite.Add(packetDuration)
if sleepFor := time.Until(nextWrite); sleepFor > 0 {
time.Sleep(sleepFor)
}

if lastChunk {
break
}
}

if remaining := holdOpen - time.Since(streamStart); remaining > 0 {
time.Sleep(remaining)
}

return nil
}

func copyFile(sourcePath, destinationPath string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		return err
	}

	return destinationFile.Close()
}

func (d *StreamingDaemon) enqueueDotmatrixVisualization(chunk *TTSChunk) {
	if !dotmatrixEnabled || chunk == nil || chunk.FilePath == "" {
		return
	}

	if err := os.MkdirAll(dotmatrixQueueDir, 0755); err != nil {
		log.Printf("WARN: Dotmatrix queue directory unavailable: %v", err)
		return
	}
	if err := os.MkdirAll(dotmatrixWavDir, 0755); err != nil {
		log.Printf("WARN: Dotmatrix wav directory unavailable: %v", err)
		return
	}

	timestamp := time.Now().UnixNano()
	queuedWavPath := filepath.Join(dotmatrixWavDir, fmt.Sprintf("%d_%d.wav", timestamp, chunk.Index))
	if err := copyFile(chunk.FilePath, queuedWavPath); err != nil {
		log.Printf("WARN: Dotmatrix wav staging failed for chunk %d: %v", chunk.Index, err)
		return
	}

	payload := map[string]interface{}{
		"wav_path":      queuedWavPath,
		"text":          chunk.Text,
		"created_at_ns": timestamp,
	}

	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		_ = os.Remove(queuedWavPath)
		log.Printf("WARN: Dotmatrix payload encoding failed for chunk %d: %v", chunk.Index, err)
		return
	}

	finalPath := filepath.Join(dotmatrixQueueDir, fmt.Sprintf("%d_%d.json", timestamp, chunk.Index))
	tempPath := finalPath + ".tmp"
	if err := os.WriteFile(tempPath, encodedPayload, 0644); err != nil {
		_ = os.Remove(queuedWavPath)
		log.Printf("WARN: Dotmatrix queue write failed for chunk %d: %v", chunk.Index, err)
		return
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath)
		_ = os.Remove(queuedWavPath)
		log.Printf("WARN: Dotmatrix queue finalize failed for chunk %d: %v", chunk.Index, err)
		return
	}
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

			slog.Info("chunk_ready", "chunk_index", chunk.Index, "time_to_ready_ms", timeToReady.Milliseconds(), "tts_endpoint", chunk.TTSEndpoint)

			// SEQUENTIAL PLAYBACK: Wait for predecessor to complete before starting
			if chunk.Predecessor != nil && chunk.Predecessor.PlayCompleted != nil {
				<-chunk.Predecessor.PlayCompleted
			}

			log.Printf("PLAY: Sequential playback chunk %d (order guaranteed)", chunk.Index)

			// Validate file existence and permissions
			fileInfo, err := os.Stat(chunk.FilePath)
			if err != nil {
				log.Printf("ERROR: Chunk %d file not accessible: %v", chunk.Index, err)
				close(chunk.PlayCompleted)
				continue
			}
			fileSize := fileInfo.Size()
			log.Printf("FILE: Chunk %d validated - size: %d bytes, path: %s", chunk.Index, fileSize, chunk.FilePath)
			d.enqueueDotmatrixVisualization(chunk)

			// Execute audio playback synchronously to maintain order
			log.Printf("AUDIO: Starting playback chunk %d - file: %s (mode: %s)", chunk.Index, chunk.FilePath, playbackMode)

			var cmd *exec.Cmd
			var stdout, stderr bytes.Buffer

			playbackStart := time.Now()
			if playbackMode == "vban" {
				// Stream for approximately the WAV duration plus a small buffer, then force cleanup if the emitter hangs.
				audioBytes := fileSize - 44
				if audioBytes < 0 {
					audioBytes = 0
				}
				expectedDuration := time.Duration(float64(audioBytes)/float64(22050*2)*float64(time.Second)) + 2*time.Second
				if expectedDuration < 3*time.Second {
					expectedDuration = 3 * time.Second
				}
				if expectedDuration > 30*time.Second {
					expectedDuration = 30 * time.Second
				}
				timeoutSeconds := int(expectedDuration.Seconds() + 0.999)
				fifoPath := chunk.FilePath + ".fifo"
				os.Remove(fifoPath)
				if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
					log.Printf("ERROR: Playback chunk %d failed to create fifo %s: %v", chunk.Index, fifoPath, err)
					close(chunk.PlayCompleted)
					continue
				}

				cmd = exec.Command("/usr/bin/timeout", fmt.Sprintf("%ds", timeoutSeconds),
					"/usr/local/bin/vban_emitter", "-i", vbanTargetIP, "-p", vbanTargetPort,
					"-s", vbanStreamName, "-b", "pipe", "-d", fifoPath,
					"-f", "16I", "-r", "22050", "-n", "1")
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				if err := cmd.Start(); err != nil {
					os.Remove(fifoPath)
					log.Printf("ERROR: Playback chunk %d failed to start vban emitter: %v", chunk.Index, err)
					close(chunk.PlayCompleted)
					continue
				}

				writerErrCh := make(chan error, 1)
				go func() {
					writerErrCh <- streamPCMToFIFORealTime(chunk.FilePath, fifoPath, time.Duration(timeoutSeconds)*time.Second)
				}()
				waitErr := cmd.Wait()
				writerErr := <-writerErrCh
				os.Remove(fifoPath)

				if writerErr != nil {
					log.Printf("ERROR: Playback chunk %d failed to feed fifo: %v", chunk.Index, writerErr)
				} else if waitErr != nil {
					if exitErr, ok := waitErr.(*exec.ExitError); ok && exitErr.ExitCode() == 124 {
						log.Printf("WARN: Playback chunk %d reached timeout after %v; forcing completion | stderr: %s | stdout: %s", chunk.Index, time.Since(playbackStart), stderr.String(), stdout.String())
					} else {
						log.Printf("ERROR: Playback chunk %d failed: %v | stderr: %s | stdout: %s", chunk.Index, waitErr, stderr.String(), stdout.String())
					}
				} else {
					playbackDuration := time.Since(playbackStart)
					log.Printf("SUCCESS: Playback chunk %d completed in %v | stderr: %s", chunk.Index, playbackDuration, stderr.String())
				}
			} else {
				// Default to aplay
				cmd = exec.Command("aplay", "-D", playbackDevice, chunk.FilePath)
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					log.Printf("ERROR: Playback chunk %d failed: %v | stderr: %s | stdout: %s", chunk.Index, err, stderr.String(), stdout.String())
				} else {
					playbackDuration := time.Since(playbackStart)
					log.Printf("SUCCESS: Playback chunk %d completed in %v | stderr: %s", chunk.Index, playbackDuration, stderr.String())
				}
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

	slog.Info("speak_request_processing", "sentences", len(sentences), "speaker", speaker, "request_timestamp", timestamp)

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
		slog.Warn("invalid_json_request", "remote_addr", r.RemoteAddr, "error", err.Error())
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(req.Sentences) == 0 {
		slog.Warn("empty_sentences_request", "remote_addr", r.RemoteAddr)
		http.Error(w, "No sentences provided", http.StatusBadRequest)
		return
	}

	slog.Info("speak_request_received", "remote_addr", r.RemoteAddr, "sentences", len(req.Sentences), "speaker", req.Speaker)

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
		"service":        getEnvDefault("HAL_SERVICE_NAME", "hal-tts"),
		"version":        getEnvDefault("HAL_SERVICE_VERSION", "0.0.0+build.0"),
		"version_core":   getEnvDefault("HAL_SERVICE_VERSION_CORE", "0.0.0"),
		"build":          getEnvDefault("HAL_SERVICE_BUILD", "0"),
		"release_id":     getEnvDefault("HAL_RELEASE_ID", "dev"),
		"source_commit":  getEnvDefault("HAL_SOURCE_COMMIT", ""),
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
		slog.Warn("invalid_client_json_request", "remote_addr", r.RemoteAddr, "error", err.Error())
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
			slog.Error("client_chunk_generation_failed", "chunk_index", chunk.Index, "error", chunk.Error.Error())
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
	host := os.Getenv("DAEMON_HOST")
	port := os.Getenv("DAEMON_PORT")
	if host != "" && port != "" {
		return host + ":" + port
	}
	return defaultBind
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys[:len(keys)-1] {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return keys[len(keys)-1]
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return time.Duration(parsed * float64(time.Second))
		}
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	return def
}

func main() {
	logOptions, err := observability.LoadOptionsFromEnv("mercury-tts")
	if err != nil {
		log.Fatal(err)
	}
	logger, closeLogger, err := observability.NewLoggerWithOptions(observability.ParseLevel(getEnvDefault("LOG_LEVEL", "info")), logOptions)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = closeLogger(ctx)
	}()
	slog.SetDefault(logger.Logger)
	log.SetFlags(0)
	log.SetOutput(observability.NewStdlibWriter(logger, slog.LevelInfo))

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
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
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
	log.Printf("STREAMING SAFE TTS daemon ready on %s (localhost access at http://localhost:8091) version=%s release=%s", bindAddr, getEnvDefault("HAL_SERVICE_VERSION", "0.0.0+build.0"), getEnvDefault("HAL_RELEASE_ID", "dev"))

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
