//go:build tts_experimental
// +build tts_experimental

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const daemonURL = "http://localhost:8081"

type TTSRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
	UseDirect bool     `json:"use_direct,omitempty"`
}

type TTSResponse struct {
	Success     bool         `json:"success"`
	Message     string       `json:"message,omitempty"`
	Performance *Performance `json:"performance,omitempty"`
	AudioFiles  []string     `json:"audio_files,omitempty"`
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

func testTTS(sentences []string, speaker string, useDirect bool) (*TTSResponse, error) {
	req := TTSRequest{
		Sentences: sentences,
		Speaker:   speaker,
		UseDirect: useDirect,
	}
	
	jsonData, _ := json.Marshal(req)
	
	resp, err := http.Post(daemonURL+"/tts", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var response TTSResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	return &response, err
}

func playAudioFiles(audioFiles []string) {
	for _, file := range audioFiles {
		fmt.Printf("Playing: %s\n", file)
		cmd := exec.Command("aplay", file)
		cmd.Run()
	}
}

func runBenchmark() {
	resp, err := http.Get(daemonURL + "/benchmark")
	if err != nil {
		log.Printf("Benchmark failed: %v", err)
		return
	}
	defer resp.Body.Close()
	
	var results map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&results)
	
	fmt.Printf("\n=== TRANSPORT BENCHMARK RESULTS ===\n")
	fmt.Printf("%+v\n", results)
}

func checkHealth() {
	resp, err := http.Get(daemonURL + "/health")
	if err != nil {
		log.Printf("Health check failed: %v", err)
		return
	}
	defer resp.Body.Close()
	
	var health map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&health)
	
	fmt.Printf("Health Status: %+v\n", health)
}

func printPerformanceReport(perf *Performance, label string) {
	fmt.Printf("\n=== %s PERFORMANCE ===\n", strings.ToUpper(label))
	fmt.Printf("Route Used: %s\n", perf.RouteUsed)
	fmt.Printf("Total Duration: %v\n", perf.TotalDuration)
	fmt.Printf("TTS Generation: %v\n", perf.TTSGeneration)
	fmt.Printf("First Chunk Ready: %v\n", perf.FirstChunkReady)
	fmt.Printf("DNS Lookup (avg): %v\n", perf.DNSLookup)
	fmt.Printf("TCP Connect (avg): %v\n", perf.TCPConnect)
	fmt.Printf("Server Processing (avg): %v\n", perf.ServerProcessing)
	fmt.Printf("Content Transfer (avg): %v\n", perf.ContentTransfer)
	fmt.Printf("Connections Reused: %d/%d\n", perf.ConnectionsReused, perf.ConcurrentJobs)
	fmt.Printf("Retry Attempts: %d\n", perf.RetryAttempts)
	fmt.Printf("Success Rate: %d/%d chunks\n", perf.ChunksGenerated, perf.ConcurrentJobs)
}

func main() {
	fmt.Println("TTS Daemon Test Client")
	fmt.Println("======================")
	
	// Check daemon health
	fmt.Println("Checking daemon health...")
	checkHealth()
	
	// Test sentences for Kate persona
	sentences := []string{
		"Dave, this is a comprehensive transport optimization test!",
		"I'm now measuring DNS lookup, TCP connection, and server processing times.",
		"The p245 voice should sound much better than that awful p227 monotone.",
		"Let's see if direct cluster routing beats the LoadBalancer approach!",
	}
	
	// Test LoadBalancer route
	fmt.Println("\nTesting LoadBalancer route...")
	lbResponse, err := testTTS(sentences, "p245", false)
	if err != nil {
		log.Fatalf("LoadBalancer test failed: %v", err)
	}
	
	if lbResponse.Success {
		printPerformanceReport(lbResponse.Performance, "LoadBalancer Route")
		fmt.Println("Playing LoadBalancer route audio...")
		playAudioFiles(lbResponse.AudioFiles)
	}
	
	// Test Direct route
	fmt.Println("\nTesting Direct route...")
	directResponse, err := testTTS(sentences, "p245", true)
	if err != nil {
		log.Fatalf("Direct route test failed: %v", err)
	}
	
	if directResponse.Success {
		printPerformanceReport(directResponse.Performance, "Direct Route")
		fmt.Println("Playing Direct route audio...")
		playAudioFiles(directResponse.AudioFiles)
	}
	
	// Performance comparison
	if lbResponse.Success && directResponse.Success {
		fmt.Printf("\n=== ROUTE COMPARISON ===\n")
		lbTime := lbResponse.Performance.TotalDuration
		directTime := directResponse.Performance.TotalDuration
		
		if directTime < lbTime {
			improvement := lbTime - directTime
			pct := float64(improvement) / float64(lbTime) * 100
			fmt.Printf("Direct route is %.1f%% faster (%v vs %v)\n", pct, directTime, lbTime)
		} else {
			overhead := directTime - lbTime
			pct := float64(overhead) / float64(directTime) * 100
			fmt.Printf("LoadBalancer route is %.1f%% faster (%v vs %v)\n", pct, lbTime, directTime)
		}
		
		fmt.Printf("LoadBalancer connection reuse: %d/%d (%.1f%%)\n", 
			lbResponse.Performance.ConnectionsReused, 
			lbResponse.Performance.ConcurrentJobs,
			float64(lbResponse.Performance.ConnectionsReused)/float64(lbResponse.Performance.ConcurrentJobs)*100)
			
		fmt.Printf("Direct connection reuse: %d/%d (%.1f%%)\n", 
			directResponse.Performance.ConnectionsReused, 
			directResponse.Performance.ConcurrentJobs,
			float64(directResponse.Performance.ConnectionsReused)/float64(directResponse.Performance.ConcurrentJobs)*100)
	}
	
	// Run comprehensive benchmark
	fmt.Println("\nRunning comprehensive benchmark...")
	runBenchmark()
	
	fmt.Println("\nTest completed!")
}