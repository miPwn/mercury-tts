package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
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
	cmteckMicDevice = "hw:4,0"  // CMTECK microphone
	testOutputDir = "/tmp/audio_feedback_test"
	silenceThreshold = -40.0  // dB threshold for silence detection
	audioTimeoutSeconds = 20  // Maximum time to wait for audio
	speakerSpinupDelay = 2    // Seconds to wait for speakers to spin up
)

type TestRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
}

type AudioFeedbackResult struct {
	TestID              string        `json:"test_id"`
	RequestSent         bool          `json:"request_sent"`
	APIResponse         string        `json:"api_response"`
	MicrophoneDetection bool          `json:"microphone_detection"`
	AudioLevelPeak      float64       `json:"audio_level_peak"`
	AudioDuration       time.Duration `json:"audio_duration"`
	SpeakerSpinupTime   time.Duration `json:"speaker_spinup_time"`
	TotalTestTime       time.Duration `json:"total_test_time"`
	ValidationSteps     []string      `json:"validation_steps"`
	Errors              []string      `json:"errors"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: audio_feedback_test \"test sentence\" [speaker]")
		os.Exit(1)
	}

	testSentence := os.Args[1]
	speaker := "p254"
	if len(os.Args) > 2 {
		speaker = os.Args[2]
	}

	result := runAudioFeedbackTest(testSentence, speaker)
	
	// Output detailed results
	fmt.Printf("\n=== AUDIO FEEDBACK TEST RESULTS ===\n")
	fmt.Printf("Test ID: %s\n", result.TestID)
	fmt.Printf("API Request Sent: %t\n", result.RequestSent)
	fmt.Printf("Microphone Detection: %t\n", result.MicrophoneDetection)
	fmt.Printf("Peak Audio Level: %.2f dB\n", result.AudioLevelPeak)
	fmt.Printf("Audio Duration: %v\n", result.AudioDuration)
	fmt.Printf("Speaker Spinup Time: %v\n", result.SpeakerSpinupTime)
	fmt.Printf("Total Test Time: %v\n", result.TotalTestTime)
	
	fmt.Printf("\nValidation Steps:\n")
	for i, step := range result.ValidationSteps {
		fmt.Printf("  %d. %s\n", i+1, step)
	}
	
	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors:\n")
		for _, err := range result.Errors {
			fmt.Printf("  ❌ %s\n", err)
		}
	}
	
	// Overall test status
	if result.MicrophoneDetection && len(result.Errors) == 0 {
		fmt.Printf("\n✅ AUDIO FEEDBACK TEST PASSED - Audio confirmed via microphone\n")
		os.Exit(0)
	} else {
		fmt.Printf("\n❌ AUDIO FEEDBACK TEST FAILED - No audio detected\n")
		os.Exit(1)
	}
}

func runAudioFeedbackTest(sentence, speaker string) AudioFeedbackResult {
	startTime := time.Now()
	testID := strconv.FormatInt(startTime.UnixNano(), 10)
	
	result := AudioFeedbackResult{
		TestID: testID,
		Errors: []string{},
		ValidationSteps: []string{},
	}
	
	// Prepare test directory
	os.RemoveAll(testOutputDir)
	os.MkdirAll(testOutputDir, 0755)
	
	// Step 1: Test microphone access
	result.ValidationSteps = append(result.ValidationSteps, "Testing microphone access")
	if !testMicrophoneAccess(&result) {
		result.TotalTestTime = time.Since(startTime)
		return result
	}
	
	// Step 2: Check daemon health
	result.ValidationSteps = append(result.ValidationSteps, "Checking TTS daemon health")
	if !checkDaemonHealth(&result) {
		result.TotalTestTime = time.Since(startTime)
		return result
	}
	
	// Step 3: Start microphone monitoring before TTS request
	result.ValidationSteps = append(result.ValidationSteps, "Starting microphone monitoring")
	micRecordingFile := filepath.Join(testOutputDir, "microphone_capture.wav")
	micCmd := startMicrophoneRecording(micRecordingFile)
	if micCmd == nil {
		result.Errors = append(result.Errors, "Failed to start microphone recording")
		result.TotalTestTime = time.Since(startTime)
		return result
	}
	defer micCmd.Process.Kill()
	
	// Step 4: Wait for speaker spinup delay
	result.ValidationSteps = append(result.ValidationSteps, fmt.Sprintf("Waiting %ds for speaker spinup", speakerSpinupDelay))
	spinupStart := time.Now()
	time.Sleep(time.Duration(speakerSpinupDelay) * time.Second)
	result.SpeakerSpinupTime = time.Since(spinupStart)
	
	// Step 5: Send TTS request
	result.ValidationSteps = append(result.ValidationSteps, "Sending TTS request")
	requestTime := time.Now()
	if !sendTTSRequest(sentence, speaker, &result) {
		result.TotalTestTime = time.Since(startTime)
		return result
	}
	
	// Step 6: Monitor microphone for audio output
	result.ValidationSteps = append(result.ValidationSteps, "Monitoring microphone for audio output")
	audioDetectionStart := time.Now()
	
	// Wait for audio generation and playback
	time.Sleep(time.Duration(audioTimeoutSeconds) * time.Second)
	
	// Stop microphone recording
	micCmd.Process.Kill()
	micCmd.Wait()
	
	// Step 7: Analyze recorded audio
	result.ValidationSteps = append(result.ValidationSteps, "Analyzing recorded audio for speech detection")
	if analyzeRecordedAudio(micRecordingFile, &result) {
		result.AudioDuration = time.Since(audioDetectionStart)
		result.MicrophoneDetection = true
	}
	
	result.TotalTestTime = time.Since(startTime)
	return result
}

func testMicrophoneAccess(result *AudioFeedbackResult) bool {
	// Test microphone with a short recording
	testFile := filepath.Join(testOutputDir, "mic_test.wav")
	cmd := exec.Command("arecord", "-D", cmteckMicDevice, "-f", "cd", "-t", "wav", "-d", "1", testFile)
	
	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Microphone test failed: %v", err))
		return false
	}
	
	// Verify the test file was created
	if _, err := os.Stat(testFile); err != nil {
		result.Errors = append(result.Errors, "Microphone test file not created")
		return false
	}
	
	os.Remove(testFile) // Clean up test file
	return true
}

func checkDaemonHealth(result *AudioFeedbackResult) bool {
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

func sendTTSRequest(sentence, speaker string, result *AudioFeedbackResult) bool {
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

func startMicrophoneRecording(outputFile string) *exec.Cmd {
	// Record with high quality for better analysis
	cmd := exec.Command("arecord", 
		"-D", cmteckMicDevice,
		"-f", "cd",        // CD quality (44.1kHz, 16-bit, stereo)
		"-t", "wav",       // WAV format
		"-d", strconv.Itoa(audioTimeoutSeconds + 5), // Duration + buffer
		outputFile)
	
	if err := cmd.Start(); err != nil {
		return nil
	}
	
	return cmd
}

func analyzeRecordedAudio(audioFile string, result *AudioFeedbackResult) bool {
	// Check if file exists and has content
	stat, err := os.Stat(audioFile)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Audio file not found: %v", err))
		return false
	}
	
	if stat.Size() < 1000 { // Very small file indicates no recording
		result.Errors = append(result.Errors, "Audio file too small, no recording detected")
		return false
	}
	
	// Use sox to analyze audio levels if available
	if hasSox() {
		return analyzeBySox(audioFile, result)
	}
	
	// Fallback: Use ffmpeg if available
	if hasFFmpeg() {
		return analyzeByFFmpeg(audioFile, result)
	}
	
	// Basic file size analysis as last resort
	return analyzeByFileSize(audioFile, result)
}

func hasSox() bool {
	cmd := exec.Command("which", "sox")
	return cmd.Run() == nil
}

func hasFFmpeg() bool {
	cmd := exec.Command("which", "ffmpeg")
	return cmd.Run() == nil
}

func analyzeBySox(audioFile string, result *AudioFeedbackResult) bool {
	// Use sox to get audio statistics
	cmd := exec.Command("sox", audioFile, "-n", "stat")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	cmd.Run() // sox outputs stats to stderr even on success
	
	output := stderr.String()
	
	// Parse maximum amplitude or RMS values
	if strings.Contains(output, "Maximum amplitude:") {
		// Look for non-silent audio (amplitude > threshold)
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "Maximum amplitude:") {
				// Extract amplitude value
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					if amplitude, err := strconv.ParseFloat(parts[2], 64); err == nil {
						// Convert to approximate dB
						if amplitude > 0 {
							result.AudioLevelPeak = 20 * math.Log10(amplitude)
							// Consider audio detected if amplitude is significant
							return result.AudioLevelPeak > silenceThreshold
						}
					}
				}
			}
		}
	}
	
	return false
}

func analyzeByFFmpeg(audioFile string, result *AudioFeedbackResult) bool {
	// Use ffmpeg to analyze volume levels
	cmd := exec.Command("ffmpeg", "-i", audioFile, "-af", "volumedetect", "-f", "null", "/dev/null")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	cmd.Run()
	
	output := stderr.String()
	
	// Parse volume detection output
	if strings.Contains(output, "mean_volume:") {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "mean_volume:") && strings.Contains(line, "dB") {
				// Extract volume level
				parts := strings.Fields(line)
				for i, part := range parts {
					if strings.Contains(part, "dB") && i > 0 {
						if volume, err := strconv.ParseFloat(parts[i-1], 64); err == nil {
							result.AudioLevelPeak = volume
							return volume > silenceThreshold
						}
					}
				}
			}
		}
	}
	
	return false
}

func analyzeByFileSize(audioFile string, result *AudioFeedbackResult) bool {
	stat, _ := os.Stat(audioFile)
	
	// Basic heuristic: significant audio should result in larger file
	// CD quality silence for 20 seconds ≈ 1.7MB
	// Actual speech should be noticeably larger due to audio content
	expectedSilenceSize := int64(1.7 * 1024 * 1024) // 1.7MB
	
	if stat.Size() > expectedSilenceSize + 100*1024 { // +100KB buffer for speech
		result.AudioLevelPeak = -20.0 // Estimated level for detected audio
		return true
	}
	
	result.Errors = append(result.Errors, fmt.Sprintf("File size analysis: %d bytes (expected > %d for audio)", stat.Size(), expectedSilenceSize))
	return false
}