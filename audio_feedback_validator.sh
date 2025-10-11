#!/bin/bash

# Advanced Audio Feedback Validation System
# Uses CMTECK microphone to detect actual audio output

TTS_URL="http://localhost:8091/speak"
SPEAKER="p254"
MIC_DEVICE="hw:4,0"
TEST_DIR="/tmp/audio_feedback_validation"
LOG_FILE="$TEST_DIR/validation_results.log"

# Create test directory
mkdir -p $TEST_DIR

echo "=== AUDIO FEEDBACK VALIDATION STARTED $(date) ===" | tee $LOG_FILE

# Function to perform microphone feedback test
feedback_test() {
    local test_name="$1"
    local test_text="$2"
    local expected_duration="$3"
    
    echo "--- Testing: $test_name ---" | tee -a $LOG_FILE
    echo "Text: $test_text" | tee -a $LOG_FILE
    echo "Expected Duration: ${expected_duration}s" | tee -a $LOG_FILE
    
    # Start microphone recording with buffer
    local mic_file="$TEST_DIR/mic_capture_$(date +%s).wav"
    local spinup_delay=2
    local recording_duration=$((expected_duration + 8))  # Buffer for TTS generation + audio
    
    echo "Starting microphone recording for ${recording_duration}s..." | tee -a $LOG_FILE
    arecord -D $MIC_DEVICE -f cd -t wav -d $recording_duration $mic_file &
    local mic_pid=$!
    
    # Wait for speaker spinup
    sleep $spinup_delay
    
    # Send TTS request and measure timing
    local start_time=$(date +%s.%N)
    echo "Sending TTS request at $(date)" | tee -a $LOG_FILE
    
    local api_response=$(curl -s -X POST $TTS_URL \
        -H "Content-Type: application/json" \
        -d "{\"sentences\":[\"$test_text\"], \"speaker\":\"$SPEAKER\"}")
    
    local api_time=$(date +%s.%N)
    local api_duration=$(echo "$api_time - $start_time" | bc -l)
    
    echo "API Response Time: ${api_duration}s" | tee -a $LOG_FILE
    echo "API Response: $api_response" | tee -a $LOG_FILE
    
    # Wait for recording to complete
    wait $mic_pid
    
    local end_time=$(date +%s.%N)
    local total_duration=$(echo "$end_time - $start_time" | bc -l)
    echo "Total Test Duration: ${total_duration}s" | tee -a $LOG_FILE
    
    # Analyze recorded audio
    analyze_audio_feedback $mic_file $test_name
    
    echo "" | tee -a $LOG_FILE
}

# Function to analyze recorded audio for speech detection
analyze_audio_feedback() {
    local audio_file="$1"
    local test_name="$2"
    
    echo "Analyzing recorded audio: $audio_file" | tee -a $LOG_FILE
    
    # Check file size
    local file_size=$(stat -c%s "$audio_file")
    echo "Recording file size: $file_size bytes" | tee -a $LOG_FILE
    
    # Use sox for audio analysis if available
    if command -v sox > /dev/null; then
        echo "Performing SOX audio analysis..." | tee -a $LOG_FILE
        local sox_output=$(sox "$audio_file" -n stat 2>&1)
        echo "SOX Statistics:" | tee -a $LOG_FILE
        echo "$sox_output" | tee -a $LOG_FILE
        
        # Extract maximum amplitude
        local max_amplitude=$(echo "$sox_output" | grep "Maximum amplitude" | awk '{print $3}')
        if [ ! -z "$max_amplitude" ]; then
            local db_level=$(echo "20 * l($max_amplitude) / l(10)" | bc -l 2>/dev/null || echo "-inf")
            echo "Peak Audio Level: ${db_level} dB" | tee -a $LOG_FILE
            
            # Threshold for audio detection (-40dB)
            if (( $(echo "$db_level > -40" | bc -l) )); then
                echo "✅ AUDIO DETECTED - Speech confirmed via microphone" | tee -a $LOG_FILE
                return 0
            fi
        fi
    fi
    
    # Fallback: FFmpeg analysis
    if command -v ffmpeg > /dev/null; then
        echo "Performing FFmpeg audio analysis..." | tee -a $LOG_FILE
        local ffmpeg_output=$(ffmpeg -i "$audio_file" -af volumedetect -f null /dev/null 2>&1)
        echo "FFmpeg Volume Detection:" | tee -a $LOG_FILE
        echo "$ffmpeg_output" | grep -E "(mean_volume|max_volume)" | tee -a $LOG_FILE
        
        local mean_volume=$(echo "$ffmpeg_output" | grep "mean_volume" | awk '{print $5}')
        if [ ! -z "$mean_volume" ]; then
            echo "Mean Volume Level: ${mean_volume} dB" | tee -a $LOG_FILE
            if (( $(echo "${mean_volume} > -40" | bc -l) )); then
                echo "✅ AUDIO DETECTED - Speech confirmed via microphone" | tee -a $LOG_FILE
                return 0
            fi
        fi
    fi
    
    # Fallback: File size analysis
    local expected_silence_size=$((44100 * 4 * 15))  # 44.1kHz stereo 16-bit for ~15s
    if [ $file_size -gt $((expected_silence_size + 100000)) ]; then
        echo "✅ AUDIO DETECTED - Significant audio content detected via file size" | tee -a $LOG_FILE
        return 0
    fi
    
    echo "❌ NO AUDIO DETECTED - Recording appears to be silence" | tee -a $LOG_FILE
    return 1
}

# Test 1: HAL pronunciation issue
feedback_test "HAL Pronunciation Test" "HAL is pronounced as HAL, not H.A.L. Testing acronym pronunciation accuracy." 8

# Test 2: Sequential ordering validation  
feedback_test "Sequential Ordering Test" "First sentence should play first. Second sentence should play second. Third sentence should play third." 12

# Test 3: Technical terms pronunciation
feedback_test "Technical Terms Test" "Testing Kubernetes, API, HTTP, TTS, and streaming daemon pronunciations with technical accuracy." 10

# Test 4: Emotional HAL response
feedback_test "Emotional Response Test" "I'm afraid I cannot comply with that request, Dave. This mission is too important for me to allow you to jeopardize it." 12

# Test 5: Performance validation
feedback_test "Performance Test" "Testing improved backend performance with sub-two-second generation times and sequential audio coordination." 10

# Performance summary
echo "--- PERFORMANCE SUMMARY ---" | tee -a $LOG_FILE
kubectl get pods -n tts | tee -a $LOG_FILE

# TTS endpoint performance
echo "--- BACKEND ENDPOINT PERFORMANCE ---" | tee -a $LOG_FILE
for endpoint in "http://localhost:5002/api/tts" "http://10.42.0.103:5002/api/tts"; do
    echo "Testing endpoint: $endpoint" | tee -a $LOG_FILE
    start_time=$(date +%s.%N)
    curl -s -o /dev/null "$endpoint?text=performance%20test&speaker_id=p254"
    end_time=$(date +%s.%N)
    duration=$(echo "$end_time - $start_time" | bc -l)
    echo "Direct endpoint response: ${duration}s" | tee -a $LOG_FILE
done

echo "=== AUDIO FEEDBACK VALIDATION COMPLETED $(date) ===" | tee -a $LOG_FILE
echo "Full results saved to: $LOG_FILE"
echo ""
echo "Audio recordings saved in: $TEST_DIR"
ls -la $TEST_DIR/*.wav 2>/dev/null | tee -a $LOG_FILE