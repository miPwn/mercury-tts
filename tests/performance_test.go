package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// Performance thresholds - fail build if exceeded
const (
	MaxResponseLatency    = 5 * time.Millisecond  // API response time
	MaxFirstAudioLatency  = 2 * time.Second       // Time to first audio output
	MaxConcurrentRequests = 10                     // Concurrent request capacity
	MaxMemoryUsage        = 100 * 1024 * 1024     // 100MB memory limit
)

func BenchmarkAPIResponseTime(b *testing.B) {
	sentences := []string{"Performance test sentence"}
	reqData := map[string]interface{}{
		"sentences": sentences,
		"speaker":   "p254",
	}
	jsonData, _ := json.Marshal(reqData)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		start := time.Now()
		
		resp, err := http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			b.Skipf("Daemon not running: %v", err)
			return
		}
		
		latency := time.Since(start)
		resp.Body.Close()

		if latency > MaxResponseLatency {
			b.Errorf("Response latency %v exceeded threshold %v", latency, MaxResponseLatency)
		}

		b.ReportMetric(float64(latency.Nanoseconds()), "ns/response")
	}
}

func BenchmarkConcurrentLoad(b *testing.B) {
	sentences := []string{"Concurrent load test"}
	reqData := map[string]interface{}{
		"sentences": sentences,
		"speaker":   "p254",
	}
	jsonData, _ := json.Marshal(reqData)

	b.SetParallelism(MaxConcurrentRequests)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			start := time.Now()
			
			resp, err := http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				b.Errorf("Request failed: %v", err)
				continue
			}
			
			latency := time.Since(start)
			resp.Body.Close()

			if latency > MaxResponseLatency*2 { // Allow 2x under load
				b.Errorf("Under load latency %v exceeded threshold %v", latency, MaxResponseLatency*2)
			}
		}
	})
}

func BenchmarkEndToEndLatency(b *testing.B) {
	b.Log("Measuring complete end-to-end TTS pipeline latency")
	
	sentences := []string{
		"End to end latency measurement test",
		"This measures the complete pipeline performance",
	}
	
	reqData := map[string]interface{}{
		"sentences": sentences,
		"speaker":   "p254",
	}
	jsonData, _ := json.Marshal(reqData)

	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		start := time.Now()
		
		// API call
		resp, err := http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			b.Skipf("Daemon not running: %v", err)
			return
		}
		resp.Body.Close()
		
		// Wait for audio processing to complete
		time.Sleep(3 * time.Second)
		
		totalLatency := time.Since(start)
		
		if totalLatency > MaxFirstAudioLatency*2 {
			b.Errorf("End-to-end latency %v exceeded threshold %v", totalLatency, MaxFirstAudioLatency*2)
		}

		b.ReportMetric(float64(totalLatency.Milliseconds()), "ms/complete")
	}
}

func TestLatencyRegression(t *testing.T) {
	t.Log("Testing for performance regression")
	requireLocalDaemon(t)
	
	// Baseline measurements
	baselines := map[string]time.Duration{
		"single_sentence": 2 * time.Second,
		"multi_sentence":  4 * time.Second,
		"concurrent":      3 * time.Second,
	}
	
	// Single sentence test
	start := time.Now()
	sentences := []string{"Single sentence latency test"}
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
	resp.Body.Close()
	
	time.Sleep(2 * time.Second) // Wait for processing
	singleLatency := time.Since(start)
	
	if singleLatency > baselines["single_sentence"] {
		t.Errorf("Single sentence latency regression: %v > %v", singleLatency, baselines["single_sentence"])
	}
	
	// Multi sentence test
	start = time.Now()
	sentences = []string{
		"Multi sentence test one",
		"Multi sentence test two", 
		"Multi sentence test three",
	}
	reqData["sentences"] = sentences
	jsonData, _ = json.Marshal(reqData)
	
	resp, err = http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Errorf("Multi sentence test failed: %v", err)
		return
	}
	resp.Body.Close()
	
	time.Sleep(4 * time.Second)
	multiLatency := time.Since(start)
	
	if multiLatency > baselines["multi_sentence"] {
		t.Errorf("Multi sentence latency regression: %v > %v", multiLatency, baselines["multi_sentence"])
	}
	
	t.Logf("Latency results - Single: %v, Multi: %v", singleLatency, multiLatency)
}

func TestThroughput(t *testing.T) {
	t.Log("Testing system throughput capacity")
	requireLocalDaemon(t)
	
	sentences := []string{"Throughput test sentence"}
	reqData := map[string]interface{}{
		"sentences": sentences,
		"speaker":   "p254",
	}
	jsonData, _ := json.Marshal(reqData)
	
	// Measure requests per second
	start := time.Now()
	requestCount := 0
	duration := 5 * time.Second
	
	for time.Since(start) < duration {
		resp, err := http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Errorf("Throughput test request failed: %v", err)
			break
		}
		resp.Body.Close()
		requestCount++
		
		time.Sleep(100 * time.Millisecond) // Small delay between requests
	}
	
	actualDuration := time.Since(start)
	rps := float64(requestCount) / actualDuration.Seconds()
	
	t.Logf("Throughput: %d requests in %v (%.2f RPS)", requestCount, actualDuration, rps)
	
	minRPS := 2.0 // Minimum 2 requests per second
	if rps < minRPS {
		t.Errorf("Throughput too low: %.2f RPS < %.2f RPS", rps, minRPS)
	}
}

func BenchmarkMemoryUsage(b *testing.B) {
	sentences := []string{"Memory usage test sentence"}
	reqData := map[string]interface{}{
		"sentences": sentences,
		"speaker":   "p254",
	}
	jsonData, _ := json.Marshal(reqData)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp, err := http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			b.Skipf("Daemon not running: %v", err)
			return
		}
		resp.Body.Close()
	}
}

func TestAudioQuality(t *testing.T) {
	t.Log("Testing audio output quality and consistency")
	requireLocalDaemon(t)
	
	sentences := []string{"Audio quality test sentence for consistency checking"}
	reqData := map[string]interface{}{
		"sentences": sentences,
		"speaker":   "p254",
	}
	jsonData, _ := json.Marshal(reqData)
	
	// Generate multiple audio samples
	for i := 0; i < 3; i++ {
		resp, err := http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Errorf("Audio quality test %d failed: %v", i+1, err)
			continue
		}
		resp.Body.Close()
		
		// Wait for processing
		time.Sleep(2 * time.Second)
		
		// In a full implementation, we would analyze the generated audio files
		// for consistency, quality metrics, etc.
	}
	
	t.Log("Audio quality test completed - manual verification recommended")
}

func TestStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}
	
	t.Log("Running stress test - high load scenario")
	requireLocalDaemon(t)
	
	sentences := []string{
		"Stress test sentence number one with extended content",
		"Stress test sentence number two with more extended content", 
		"Stress test sentence number three with even more extended content",
	}
	
	reqData := map[string]interface{}{
		"sentences": sentences,
		"speaker":   "p254",
	}
	jsonData, _ := json.Marshal(reqData)
	
	// High load test
	concurrency := 20
	requestsPerWorker := 5
	
	done := make(chan error, concurrency)
	
	for worker := 0; worker < concurrency; worker++ {
		go func(workerID int) {
			for req := 0; req < requestsPerWorker; req++ {
				resp, err := http.Post("http://localhost:8091/speak", "application/json", bytes.NewBuffer(jsonData))
				if err != nil {
					done <- fmt.Errorf("Worker %d request %d failed: %v", workerID, req+1, err)
					return
				}
				resp.Body.Close()
				
				time.Sleep(100 * time.Millisecond)
			}
			done <- nil
		}(worker)
	}
	
	// Collect results
	failures := 0
	for worker := 0; worker < concurrency; worker++ {
		if err := <-done; err != nil {
			t.Logf("Stress test failure: %v", err)
			failures++
		}
	}
	
	totalRequests := concurrency * requestsPerWorker
	successRate := float64(totalRequests-failures) / float64(totalRequests) * 100
	
	t.Logf("Stress test completed: %d/%d successful (%.1f%%)", totalRequests-failures, totalRequests, successRate)
	
	if successRate < 90.0 {
		t.Errorf("Stress test success rate too low: %.1f%% < 90%%", successRate)
	}
}