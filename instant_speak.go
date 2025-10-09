package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const daemonURL = "http://localhost:8081"

type SpeakRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
}

type TTSResponse struct {
	Success    bool     `json:"success"`
	Message    string   `json:"message,omitempty"`
	AudioFiles []string `json:"audio_files,omitempty"`
}

func speak(sentences []string, speaker string) {
	req := SpeakRequest{
		Sentences: sentences,
		Speaker:   speaker,
	}
	
	jsonData, _ := json.Marshal(req)
	
	start := time.Now()
	resp, err := http.Post(daemonURL+"/tts", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	duration := time.Since(start)
	
	var response TTSResponse
	json.NewDecoder(resp.Body).Decode(&response)
	
	fmt.Printf("Response in %v: %v\n", duration, response.Message)
	
	// Play each generated audio file
	for _, audioFile := range response.AudioFiles {
		playURL := fmt.Sprintf("%s/play?file=%s", daemonURL, audioFile)
		_, err := http.Get(playURL)
		if err != nil {
			fmt.Printf("Error playing %s: %v\n", audioFile, err)
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./instant_speak \"sentence1\" \"sentence2\" ...")
		os.Exit(1)
	}
	
	sentences := os.Args[1:]
	speak(sentences, "p254")
}