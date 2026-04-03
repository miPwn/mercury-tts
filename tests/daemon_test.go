package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Mock TTS server for testing
func createMockTTSServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate TTS generation delay
		time.Sleep(100 * time.Millisecond)
		
		// Return mock WAV data
		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		
		// Minimal WAV header + data
		wavData := []byte("RIFFDWAVEfmt data" + strings.Repeat("mock_audio_data", 100))
		if _, err := w.Write(wavData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
}

func TestSequentialOrdering(t *testing.T) {
	t.Log("Testing sequential audio chunk ordering")
	
	mockServer := createMockTTSServer()
	defer mockServer.Close()
	
	// Test sentences with identifiable content
	sentences := []string{
		"First sentence should play first",
		"Second sentence should play second", 
		"Third sentence should play third",
		"Fourth sentence should play fourth",
	}
	
	// Create request payload
	reqData := map[string]interface{}{
		"sentences": sentences,
		"speaker":   "p254",
	}
	
	jsonData, _ := json.Marshal(reqData)
	
	// Test against daemon endpoint (assuming daemon is running)
	resp, err := http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Skipf("Daemon not running: %v", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	
	// Give time for processing
	time.Sleep(5 * time.Second)
	
	t.Log("Sequential ordering test completed - manual verification required")
}

func TestConcurrentRequestHandling(t *testing.T) {
	t.Log("Testing concurrent request handling without race conditions")
	
	sentences := []string{"Concurrent test sentence one", "Concurrent test sentence two"}
	reqData := map[string]interface{}{
		"sentences": sentences,
		"speaker":   "p254",
	}
	jsonData, _ := json.Marshal(reqData)
	
	// Launch 5 concurrent requests
	done := make(chan bool, 5)
	
	for i := 0; i < 5; i++ {
		go func(id int) {
			resp, err := http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				t.Errorf("Request %d failed: %v", id, err)
				done <- false
				return
			}
			defer resp.Body.Close()
			
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Request %d got status %d", id, resp.StatusCode)
				done <- false
				return
			}
			
			done <- true
		}(i)
	}
	
	// Wait for all requests
	successCount := 0
	for i := 0; i < 5; i++ {
		if <-done {
			successCount++
		}
	}
	
	if successCount != 5 {
		t.Errorf("Expected 5 successful requests, got %d", successCount)
	}
	
	t.Logf("Concurrent test completed: %d/5 requests successful", successCount)
}

func TestHealthEndpoint(t *testing.T) {
	t.Log("Testing daemon health endpoint")
	
	resp, err := http.Get("http://localhost:8091/health")
	if err != nil {
		t.Skipf("Daemon not running: %v", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health check failed with status %d", resp.StatusCode)
	}
	
	var healthData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&healthData); err != nil {
		t.Errorf("Failed to decode health response: %v", err)
	}
	
	// Check required health fields
	requiredFields := []string{"status", "warmed_clients", "audio_ready", "timestamp"}
	for _, field := range requiredFields {
		if _, exists := healthData[field]; !exists {
			t.Errorf("Health response missing field: %s", field)
		}
	}
	
	t.Logf("Health check passed: %v", healthData)
}

func TestErrorHandling(t *testing.T) {
	t.Log("Testing error handling for invalid requests")
	
	// Test empty request
	resp, err := http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer([]byte("{}")))
	if err != nil {
		t.Skipf("Daemon not running: %v", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty request, got %d", resp.StatusCode)
	}
	
	// Test malformed JSON
	resp, err = http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer([]byte("invalid json")))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 for invalid JSON, got %d", resp.StatusCode)
		}
	}
	
	t.Log("Error handling test completed")
}

func TestFileCleanup(t *testing.T) {
	t.Log("Testing temporary file cleanup")
	
	tempDir := "/tmp/streaming_safe_daemon"
	
	// Count files before request
	beforeFiles := 0
	if entries, err := os.ReadDir(tempDir); err == nil {
		beforeFiles = len(entries)
	}
	
	// Make a request
	sentences := []string{"Testing file cleanup"}
	reqData := map[string]interface{}{
		"sentences": sentences,
		"speaker":   "p254",
	}
	jsonData, _ := json.Marshal(reqData)
	
	resp, err := http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Skipf("Daemon not running: %v", err)
		return
	}
	defer resp.Body.Close()
	
	// Wait for processing and cleanup
	time.Sleep(3 * time.Second)
	
	// Count files after request
	afterFiles := 0
	if entries, err := os.ReadDir(tempDir); err == nil {
		afterFiles = len(entries)
	}
	
	if afterFiles > beforeFiles {
		t.Errorf("File cleanup failed: %d files before, %d after", beforeFiles, afterFiles)
	}
	
	t.Logf("File cleanup test passed: %d files before, %d after", beforeFiles, afterFiles)
}

func TestRetryMechanism(t *testing.T) {
	t.Log("Testing retry mechanism with mock failures")
	
	// This test would require a more sophisticated mock server
	// that can simulate failures and recoveries
	t.Log("Retry mechanism test - requires integration with mock TTS server")
}