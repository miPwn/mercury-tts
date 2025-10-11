package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	ttsURL     = "http://localhost:5002/api/tts"
	speaker    = "p260"  // Changed to p260 for more emotional range
	maxRetries = 3
)

type TTSChunk struct {
	Index    int
	Text     string
	FilePath string
	Ready    chan bool
	Error    error
}

type TTSStreamer struct {
	client   *http.Client
	outputDir string
	timestamp string
}

func NewTTSStreamer() *TTSStreamer {
	// HTTP client with connection pooling and keep-alive
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     60 * time.Second,
			DisableKeepAlives:   false,
		},
	}

	outputDir := "/tmp/tts_go_streaming"
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)

	return &TTSStreamer{
		client:    client,
		outputDir: outputDir,
		timestamp: timestamp,
	}
}

func (tts *TTSStreamer) preWarmService() {
	// Send a dummy request to warm up the TTS service
	go func() {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s?text=warm&speaker_id=%s", ttsURL, speaker), nil)
		resp, err := tts.client.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}()
}

func (tts *TTSStreamer) generateTTS(chunk *TTSChunk, wg *sync.WaitGroup) {
	defer wg.Done()
	
	encodedText := url.QueryEscape(chunk.Text)
	requestURL := fmt.Sprintf("%s?text=%s&speaker_id=%s", ttsURL, encodedText, speaker)
	
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = tts.downloadAudio(requestURL, chunk.FilePath)
		if err == nil {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}
	
	chunk.Error = err
	close(chunk.Ready) // Signal completion
}

func (tts *TTSStreamer) downloadAudio(requestURL, filePath string) error {
	resp, err := tts.client.Get(requestURL)
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

func (tts *TTSStreamer) playAudio(filePath string) error {
	cmd := exec.Command("aplay", filePath)
	cmd.Stderr = nil // Suppress aplay output
	return cmd.Run()
}

func (tts *TTSStreamer) streamPlayback(chunks []*TTSChunk) {
	for _, chunk := range chunks {
		// Wait for this chunk to be ready
		<-chunk.Ready
		
		if chunk.Error != nil {
			fmt.Printf("Error generating chunk %d: %v\n", chunk.Index, chunk.Error)
			continue
		}
		
		fmt.Printf("Streaming chunk %d...\n", chunk.Index)
		if err := tts.playAudio(chunk.FilePath); err != nil {
			fmt.Printf("Error playing chunk %d: %v\n", chunk.Index, err)
		}
	}
}

func (tts *TTSStreamer) Process(sentences []string) {
	fmt.Println("Launching high-performance Go TTS streaming...")
	
	// Pre-warm the service
	tts.preWarmService()
	
	// Create chunks
	chunks := make([]*TTSChunk, len(sentences))
	for i, sentence := range sentences {
		chunks[i] = &TTSChunk{
			Index:    i + 1,
			Text:     sentence,
			FilePath: filepath.Join(tts.outputDir, fmt.Sprintf("%s_%d.wav", tts.timestamp, i+1)),
			Ready:    make(chan bool),
		}
	}
	
	// Start all TTS generation goroutines
	var wg sync.WaitGroup
	for _, chunk := range chunks {
		wg.Add(1)
		go tts.generateTTS(chunk, &wg)
	}
	
	// Start streaming playback immediately
	go tts.streamPlayback(chunks)
	
	// Wait for all generation to complete
	wg.Wait()
	
	// Brief pause to ensure final audio finishes
	time.Sleep(100 * time.Millisecond)
	
	// Cleanup
	os.RemoveAll(tts.outputDir)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./tts_streamer \"sentence1\" \"sentence2\" ...")
		os.Exit(1)
	}
	
	sentences := os.Args[1:]
	streamer := NewTTSStreamer()
	streamer.Process(sentences)
}