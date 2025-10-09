package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Remote TTS daemon endpoint - use falcon.mipwn.local or IP fallback
const (
	remoteTTSHost = "192.168.1.106" // falcon.mipwn.local
	remoteTTSPort = "8091"
	remoteTTSURL  = "http://" + remoteTTSHost + ":" + remoteTTSPort
)

type SpeakRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
}

type SpeakResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func remoteTTSSpeak(sentences []string, speaker string) error {
	if speaker == "" {
		speaker = "p254" // Default to HAL voice
	}

	req := SpeakRequest{
		Sentences: sentences,
		Speaker:   speaker,
	}

	jsonData, _ := json.Marshal(req)

	start := time.Now()
	resp, err := http.Post(remoteTTSURL+"/speak", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to connect to remote TTS daemon at %s: %v", remoteTTSURL, err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	var response SpeakResponse
	json.NewDecoder(resp.Body).Decode(&response)

	fmt.Printf("Remote TTS request sent in %v: %s\n", duration, response.Message)
	return nil
}

func checkRemoteTTSHealth() error {
	resp, err := http.Get(remoteTTSURL + "/health")
	if err != nil {
		return fmt.Errorf("remote TTS daemon not accessible at %s: %v", remoteTTSURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote TTS daemon health check failed: HTTP %d", resp.StatusCode)
	}

	var health map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&health)

	fmt.Printf("Remote TTS daemon status: %v (warmed clients: %.0f)\n", 
		health["status"], health["warmed_clients"])
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./remote_speak \"sentence1\" \"sentence2\" ...")
		fmt.Println("       ./remote_speak --health")
		fmt.Printf("Connects to remote TTS daemon at: %s\n", remoteTTSURL)
		os.Exit(1)
	}

	// Health check command
	if os.Args[1] == "--health" {
		if err := checkRemoteTTSHealth(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Regular TTS command
	sentences := os.Args[1:]
	if err := remoteTTSSpeak(sentences, "p254"); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}