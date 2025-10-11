#!/bin/bash

# Comprehensive TTS Quality Test
# Tests pronunciation, timing, and quality issues

TTS_URL="http://localhost:8091/speak"
SPEAKER="p254"
LOG_FILE="/tmp/tts_quality_test.log"

echo "=== TTS QUALITY ANALYSIS STARTED $(date) ===" | tee $LOG_FILE

# Function to send TTS request and measure timing
test_tts() {
    local test_name="$1"
    local text="$2"
    
    echo "--- Testing: $test_name ---" | tee -a $LOG_FILE
    echo "Text: $text" | tee -a $LOG_FILE
    
    start_time=$(date +%s.%N)
    
    response=$(curl -s -X POST $TTS_URL \
        -H "Content-Type: application/json" \
        -d "{\"sentences\":[\"$text\"], \"speaker\":\"$SPEAKER\"}")
    
    api_time=$(date +%s.%N)
    api_duration=$(echo "$api_time - $start_time" | bc -l)
    
    echo "API Response Time: ${api_duration}s" | tee -a $LOG_FILE
    echo "Response: $response" | tee -a $LOG_FILE
    
    # Wait for audio completion and check logs
    sleep 8
    
    end_time=$(date +%s.%N)
    total_duration=$(echo "$end_time - $start_time" | bc -l)
    echo "Total Test Duration: ${total_duration}s" | tee -a $LOG_FILE
    
    # Extract recent daemon logs
    journalctl -u hal-tts.service --no-pager -n 5 --since "10 seconds ago" | tail -3 | tee -a $LOG_FILE
    
    echo "" | tee -a $LOG_FILE
}

# Test 1: Acronym pronunciation issues
test_tts "Acronym Pronunciation" "HAL is an acronym, not H.A.L. Also test NASA, FBI, and CPU pronunciations."

# Test 2: Sequential ordering
test_tts "Sequential Ordering" "First chunk plays first. Second chunk plays second. Third chunk plays third. Fourth chunk plays fourth."

# Test 3: Technical terms
test_tts "Technical Terms" "Testing Kubernetes, Docker, API, HTTP, TTS, and streaming daemon pronunciations."

# Test 4: Numbers and measurements  
test_tts "Numbers and Units" "Testing 5ms response time, 22050 Hz sample rate, and 16-bit audio quality."

# Test 5: Long sentence stress test
test_tts "Long Sentence Stress" "This is a comprehensive stress test with a very long sentence designed to evaluate the system's capability to handle extended audio generation while maintaining sequential ordering and proper pronunciation of technical terms like Kubernetes, TTS, API endpoints, and HAL 9000 operations."

# Test 6: Emotional context
test_tts "Emotional Response" "I'm afraid I cannot comply with that request, Dave. This mission is too important for me to allow you to jeopardize it."

# Test 7: Special characters and punctuation
test_tts "Special Characters" "Testing punctuation: Hello, Dave! How are you? I'm operating at 99.9% efficiency... Fascinating."

# Backend performance check
echo "--- BACKEND ANALYSIS ---" | tee -a $LOG_FILE
kubectl get pods -n tts -o wide | tee -a $LOG_FILE

echo "--- TTS ENDPOINT TESTS ---" | tee -a $LOG_FILE
# Test direct TTS endpoints
for endpoint in "http://localhost:5002/api/tts" "http://10.42.0.103:5002/api/tts"; do
    echo "Testing endpoint: $endpoint" | tee -a $LOG_FILE
    start=$(date +%s.%N)
    response=$(curl -s -w "%{time_total}" -o /dev/null "$endpoint?text=test&speaker_id=p254")
    echo "Direct endpoint response time: ${response}s" | tee -a $LOG_FILE
done

echo "=== TTS QUALITY ANALYSIS COMPLETED $(date) ===" | tee -a $LOG_FILE
echo "Full log saved to: $LOG_FILE"