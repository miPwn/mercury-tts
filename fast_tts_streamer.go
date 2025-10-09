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
	ttsURL         = "http://192.168.1.106:5002/api/tts"
	defaultSpeaker = "p245"
	dragThreshold  = 3 * time.Second
)

type TTSChunk struct {
	Index     int
	Text      string
	FilePath  string
	Ready     chan bool
	StartTime time.Time
	EndTime   time.Time
	Error     error
}

func downloadTTS(chunk *TTSChunk, speaker string, wg *sync.WaitGroup) {
	defer wg.Done()
	
	chunk.StartTime = time.Now()
	encodedText := url.QueryEscape(chunk.Text)
	requestURL := fmt.Sprintf("%s?text=%s&speaker_id=%s", ttsURL, encodedText, speaker)
	
	resp, err := http.Get(requestURL)
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
	
	// Log if this chunk took too long
	duration := chunk.EndTime.Sub(chunk.StartTime)
	if duration > dragThreshold {
		fmt.Printf("DRAG DETECTED: Chunk %d took %v (threshold: %v)\n", chunk.Index, duration, dragThreshold)
	}
	
	close(chunk.Ready)
}

func playAudio(filePath string) error {
	cmd := exec.Command("aplay", filePath)
	cmd.Stderr = nil
	return cmd.Run()
}

func streamTTS(sentences []string, speaker string) {
	if speaker == "" {
		speaker = defaultSpeaker
	}
	
	startTime := time.Now()
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	outputDir := "/tmp/fast_tts_cache"
	
	// Setup
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)
	
	fmt.Printf("Starting concurrent TTS for %d sentences...\n", len(sentences))
	
	// Create chunks
	chunks := make([]*TTSChunk, len(sentences))
	for i, sentence := range sentences {
		chunks[i] = &TTSChunk{
			Index:    i + 1,
			Text:     sentence,
			FilePath: filepath.Join(outputDir, fmt.Sprintf("%s_%d.wav", timestamp, i+1)),
			Ready:    make(chan bool),
		}
	}
	
	// Launch all TTS requests concurrently
	var wg sync.WaitGroup
	ttsStart := time.Now()
	
	for _, chunk := range chunks {
		wg.Add(1)
		go downloadTTS(chunk, speaker, &wg)
	}
	
	// Stream playback as chunks become ready
	go func() {
		for _, chunk := range chunks {
			// Wait for this chunk to be ready
			<-chunk.Ready
			
			if chunk.Error != nil {
				fmt.Printf("ERROR: Chunk %d failed: %v\n", chunk.Index, chunk.Error)
				continue
			}
			
			// Calculate time to ready
			timeToReady := chunk.EndTime.Sub(ttsStart)
			fmt.Printf("Playing chunk %d (ready in %v)\n", chunk.Index, timeToReady)
			
			// Play immediately
			if err := playAudio(chunk.FilePath); err != nil {
				fmt.Printf("ERROR: Failed to play chunk %d: %v\n", chunk.Index, err)
			}
		}
	}()
	
	// Wait for all downloads to complete
	wg.Wait()
	
	// Brief pause to ensure final audio finishes
	time.Sleep(500 * time.Millisecond)
	
	totalTime := time.Since(startTime)
	fmt.Printf("Total time: %v\n", totalTime)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./fast_tts_streamer \"sentence1\" \"sentence2\" ...")
		os.Exit(1)
	}
	
	sentences := os.Args[1:]
	streamTTS(sentences, defaultSpeaker)
}