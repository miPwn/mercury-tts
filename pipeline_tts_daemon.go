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
	"syscall"
	"time"
)

const (
	ttsURL         = "http://localhost:5002/api/tts"
	defaultSpeaker = "p245"
	dragThreshold  = 3 * time.Second
	serverPort     = ":8091"
)

type SpeakRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
}

type TTSJob struct {
	Index     int
	Text      string
	Speaker   string
	FilePath  string
	Completed chan bool
	Error     error
	StartTime time.Time
	EndTime   time.Time
}

type PipelineTTSDaemon struct {
	client        *http.Client
	outputDir     string
	playbackReady chan bool
	jobQueue      chan *TTSJob
	playQueue     chan *TTSJob
}

func NewPipelineTTSDaemon() *PipelineTTSDaemon {
	outputDir := "/tmp/pipeline_tts_daemon"
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)

	daemon := &PipelineTTSDaemon{
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          5,
				MaxIdleConnsPerHost:   2,
				IdleConnTimeout:       30 * time.Second,
				DisableKeepAlives:     false,
				ResponseHeaderTimeout: 8 * time.Second,
			},
		},
		outputDir:     outputDir,
		playbackReady: make(chan bool, 1),
		jobQueue:      make(chan *TTSJob, 100), // Large buffer for jobs
		playQueue:     make(chan *TTSJob, 100), // Large buffer for playback
	}

	// Start the processing pipeline
	daemon.startPipeline()
	
	return daemon
}

func (d *PipelineTTSDaemon) startPipeline() {
	log.Println("Starting zero-latency TTS pipeline...")
	
	// Single TTS worker - processes jobs sequentially to avoid overloading TTS service
	go d.ttsWorker()
	
	// Playback worker - plays audio as soon as it's ready
	go d.playbackWorker()
	
	// Pre-warm system
	d.preWarmSystem()
}

func (d *PipelineTTSDaemon) preWarmSystem() {
	// Simple, fast warmup
	resp, err := d.client.Get(fmt.Sprintf("%s?text=ready&speaker_id=%s", ttsURL, defaultSpeaker))
	if err == nil && resp != nil {
		resp.Body.Close()
	}
	
	d.playbackReady <- true
	log.Println("Pipeline ready for zero-latency processing")
}

func (d *PipelineTTSDaemon) ttsWorker() {
	log.Println("TTS worker started - processing jobs sequentially")
	
	for job := range d.jobQueue {
		job.StartTime = time.Now()
		
		// Process this job immediately
		encodedText := url.QueryEscape(job.Text)
		requestURL := fmt.Sprintf("%s?text=%s&speaker_id=%s", ttsURL, encodedText, job.Speaker)
		
		resp, err := d.client.Get(requestURL)
		if err != nil {
			job.Error = err
			job.EndTime = time.Now()
			close(job.Completed)
			continue
		}
		
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			job.Error = fmt.Errorf("HTTP %d", resp.StatusCode)
			job.EndTime = time.Now()
			close(job.Completed)
			continue
		}
		
		// Save audio file
		file, err := os.Create(job.FilePath)
		if err != nil {
			resp.Body.Close()
			job.Error = err
			job.EndTime = time.Now()
			close(job.Completed)
			continue
		}
		
		_, err = io.Copy(file, resp.Body)
		file.Close()
		resp.Body.Close()
		
		job.Error = err
		job.EndTime = time.Now()
		
		// Log performance
		duration := job.EndTime.Sub(job.StartTime)
		if duration > dragThreshold {
			log.Printf("DRAG: Job %d took %v", job.Index, duration)
		} else {
			log.Printf("FAST: Job %d completed in %v", job.Index, duration)
		}
		
		// Send to playback queue immediately when ready
		d.playQueue <- job
		close(job.Completed)
	}
}

func (d *PipelineTTSDaemon) playbackWorker() {
	log.Println("Playback worker started - streaming audio immediately")
	
	for job := range d.playQueue {
		if job.Error != nil {
			log.Printf("SKIP: Job %d failed: %v", job.Index, job.Error)
			continue
		}
		
		log.Printf("PLAY: Job %d streaming now", job.Index)
		
		// Play immediately
		cmd := exec.Command("aplay", job.FilePath)
		cmd.Stderr = nil
		if err := cmd.Run(); err != nil {
			log.Printf("PLAYBACK ERROR: Job %d: %v", job.Index, err)
		}
		
		// Clean up
		os.Remove(job.FilePath)
		log.Printf("DONE: Job %d completed and cleaned up", job.Index)
	}
}

func (d *PipelineTTSDaemon) processSpeak(sentences []string, speaker string) {
	if speaker == "" {
		speaker = defaultSpeaker
	}
	
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	log.Printf("PIPELINE: Queuing %d jobs for immediate processing", len(sentences))
	
	// Create jobs and queue them immediately - NO WAITING
	for i, sentence := range sentences {
		job := &TTSJob{
			Index:     i + 1,
			Text:      sentence,
			Speaker:   speaker,
			FilePath:  filepath.Join(d.outputDir, fmt.Sprintf("%s_%d.wav", timestamp, i+1)),
			Completed: make(chan bool),
		}
		
		// Queue job immediately - this is INSTANT
		select {
		case d.jobQueue <- job:
			log.Printf("QUEUED: Job %d queued instantly", job.Index)
		default:
			log.Printf("QUEUE FULL: Job %d dropped", job.Index)
		}
	}
	
	log.Printf("PIPELINE: All %d jobs queued for sequential processing", len(sentences))
}

func (d *PipelineTTSDaemon) handleSpeak(w http.ResponseWriter, r *http.Request) {
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
	
	// Process immediately - ZERO latency
	go d.processSpeak(req.Sentences, req.Speaker)
	
	// Instant response
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("PIPELINE: %d jobs queued for zero-latency processing", len(req.Sentences)),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (d *PipelineTTSDaemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":         "pipeline-ready",
		"job_queue_len":  len(d.jobQueue),
		"play_queue_len": len(d.playQueue),
		"audio_ready":    len(d.playbackReady) > 0,
		"timestamp":      time.Now().Unix(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func main() {
	daemon := NewPipelineTTSDaemon()
	
	http.HandleFunc("/speak", daemon.handleSpeak)
	http.HandleFunc("/health", daemon.handleHealth)
	
	server := &http.Server{
		Addr:         serverPort,
		Handler:      nil,
		ReadTimeout:  2 * time.Second,  // Fast request handling
		WriteTimeout: 2 * time.Second,  // Fast response
		IdleTimeout:  30 * time.Second,
	}
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		log.Println("Shutting down pipeline TTS daemon...")
		close(daemon.jobQueue)
		close(daemon.playQueue)
		server.Close()
		os.RemoveAll(daemon.outputDir)
	}()
	
	log.Printf("ZERO-LATENCY PIPELINE TTS daemon ready on port %s", serverPort)
	log.Println("Architecture: Sequential TTS processing + Immediate streaming playback")
	
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}