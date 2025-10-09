package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const daemonURL = "http://localhost:8091"

type SpeakRequest struct {
	Sentences []string `json:"sentences"`
	Speaker   string   `json:"speaker,omitempty"`
}

type SpeakResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func speak(sentences []string, speaker string) error {
	req := SpeakRequest{
		Sentences: sentences,
		Speaker:   speaker,
	}
	
	jsonData, _ := json.Marshal(req)
	
	start := time.Now()
	resp, err := http.Post(daemonURL+"/speak", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	duration := time.Since(start)
	
	var response SpeakResponse
	json.NewDecoder(resp.Body).Decode(&response)
	
	fmt.Printf("Request sent in %v: %s\n", duration, response.Message)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./speak_client \"sentence1\" \"sentence2\" ...")
		os.Exit(1)
	}
	
	sentences := os.Args[1:]
	if err := speak(sentences, "p254"); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}