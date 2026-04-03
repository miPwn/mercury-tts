//go:build tts_experimental
// +build tts_experimental

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	daemonURL = "http://localhost:8091"
	testOutputDir = "/tmp/audio_test_validation"
)

type TestRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
}

type TestResult struct {
	TestID           string        `json:"test_id"`
	RequestSent      bool          `json:"request_sent"`
	APIResponse      string        `json:"api_response"`
	AudioFilesFound  int           `json:"audio_files_found"`
	AudioValidation  []AudioFile   `json:"audio_validation"`
	PlaybackAttempts int           `json:"playback_attempts"`
	PlaybackSuccess  bool          `json:"playback_success"`
	TotalDuration    time.Duration `json:"total_duration"`
	Errors           []string      `json:"errors"`
}

type AudioFile struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Duration string `json:"duration"`
	Valid    bool   `json:"valid"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: audio_pipeline_test \"test sentence\" [speaker]")
		os.Exit(1)
	}

	testSentence := os.Args[1]
	speaker := "p254"
	if len(os.Args) > 2 {
		speaker = os.Args[2]
	}

	result := runEndToEndTest(testSentence, speaker)
	
	// Output detailed results
	fmt.Printf("\n=== AUDIO PIPELINE TEST RESULTS ===\n")
	fmt.Printf("Test ID: %s\n", result.TestID)
	fmt.Printf("API Request Sent: %t\n", result.RequestSent)
	fmt.Printf("API Response: %s\n", result.APIResponse)
	fmt.Printf("Audio Files Generated: %d\n", result.AudioFilesFound)
	fmt.Printf("Playback Success: %t\n", result.PlaybackSuccess)
	fmt.Printf("Total Test Duration: %v\n", result.TotalDuration)
	
	if len(result.AudioValidation) > 0 {
		fmt.Printf("\nAudio File Validation:\n")
		for i, audio := range result.AudioValidation {
			fmt.Printf("  File %d: %s (size: %d bytes, duration: %s, valid: %t)\n", 
				i+1, audio.Path, audio.Size, audio.Duration, audio.Valid)
		}
	}
	
	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors Detected:\n")
		for _, err := range result.Errors {
			fmt.Printf("  - %s\n", err)
		}
	}
	
	// Overall test status
	if result.PlaybackSuccess && len(result.Errors) == 0 {
		fmt.Printf("\n✅ END-TO-END TEST PASSED\n")
		os.Exit(0)
	} else {
		fmt.Printf("\n❌ END-TO-END TEST FAILED\n")
		os.Exit(1)
	}
}

func runEndToEndTest(sentence, speaker string) TestResult {
	startTime := time.Now()
	testID := strconv.FormatInt(startTime.UnixNano(), 10)
	
	result := TestResult{
		TestID: testID,
		Errors: []string{},
	}
	
	// Prepare test directory
	os.RemoveAll(testOutputDir)
	os.MkdirAll(testOutputDir, 0755)
	
	// Step 1: Check daemon health
	if !checkDaemonHealth(&result) {
		result.TotalDuration = time.Since(startTime)
		return result
	}
	
	// Step 2: Monitor temp directory before request
	tempDir := "/tmp/streaming_safe_daemon"
	beforeFiles := getAudioFiles(tempDir)
	
	// Step 3: Send TTS request
	if !sendTTSRequest(sentence, speaker, &result) {
		result.TotalDuration = time.Since(startTime)
		return result
	}
	
	// Step 4: Wait for audio generation and monitor files
	time.Sleep(2 * time.Second) // Allow generation to start
	
	// Step 5: Monitor for new audio files
	maxWait := 15 * time.Second
	waitStart := time.Now()
	var newFiles []string
	
	for time.Since(waitStart) < maxWait {
		currentFiles := getAudioFiles(tempDir)
		newFiles = difference(currentFiles, beforeFiles)
		
		if len(newFiles) > 0 {
			// Wait a bit more for generation to complete
			time.Sleep(3 * time.Second)
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	
	// Step 6: Validate generated audio files
	result.AudioFilesFound = len(newFiles)
	if len(newFiles) > 0 {
		validateAudioFiles(newFiles, &result)
	} else {
		result.Errors = append(result.Errors, "No audio files generated")
	}
	
	// Step 7: Test direct audio playback
	if len(newFiles) > 0 {
		testAudioPlayback(newFiles[0], &result)
	}
	
	result.TotalDuration = time.Since(startTime)
	return result
}

func checkDaemonHealth(result *TestResult) bool {
	resp, err := http.Get(daemonURL + "/health")
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Health check failed: %v", err))
		return false
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		result.Errors = append(result.Errors, fmt.Sprintf("Health check returned %d", resp.StatusCode))
		return false
	}
	
	return true
}

func sendTTSRequest(sentence, speaker string, result *TestResult) bool {
	request := TestRequest{
		Sentences: []string{sentence},
		Speaker:   speaker,
	}
	
	jsonData, err := json.Marshal(request)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("JSON marshal error: %v", err))
		return false
	}
	
	resp, err := http.Post(daemonURL+"/speak", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("API request failed: %v", err))
		return false
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	result.APIResponse = string(body)
	result.RequestSent = true
	
	if resp.StatusCode != 200 {
		result.Errors = append(result.Errors, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
		return false
	}
	
	return true
}

func getAudioFiles(dir string) []string {
	var files []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(info.Name()), ".wav") {
			files = append(files, path)
		}
		return nil
	})
	return files
}

func difference(a, b []string) []string {
	mb := make(map[string]bool, len(b))
	for _, x := range b {
		mb[x] = true
	}
	
	var diff []string
	for _, x := range a {
		if !mb[x] {
			diff = append(diff, x)
		}
	}
	return diff
}

func validateAudioFiles(files []string, result *TestResult) {
	for _, file := range files {
		audio := AudioFile{Path: file}
		
		// Get file size
		if stat, err := os.Stat(file); err == nil {
			audio.Size = stat.Size()
		}
		
		// Validate file format using file command
		if output, err := exec.Command("file", file).Output(); err == nil {
			if strings.Contains(string(output), "WAVE") {
				audio.Valid = true
			}
		}
		
		// Get audio duration using soxi if available
		if output, err := exec.Command("soxi", "-D", file).Output(); err == nil {
			audio.Duration = strings.TrimSpace(string(output)) + "s"
		} else if audio.Valid {
			// Estimate duration from file size (rough calculation for 22050 Hz mono 16-bit)
			estimatedDuration := float64(audio.Size) / (22050 * 2) // bytes / (sample_rate * bytes_per_sample)
			audio.Duration = fmt.Sprintf("~%.2fs", estimatedDuration)
		}
		
		result.AudioValidation = append(result.AudioValidation, audio)
	}
}

func testAudioPlayback(audioFile string, result *TestResult) {
	result.PlaybackAttempts++
	
	// Test if aplay can play the file
	cmd := exec.Command("aplay", "-D", "default", "--test-format", audioFile)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Audio playback test failed: %v | stderr: %s", err, stderr.String()))
		return
	}
	
	// If test format succeeded, the file should be playable
	result.PlaybackSuccess = true
}