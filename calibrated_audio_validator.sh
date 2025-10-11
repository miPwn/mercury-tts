#!/bin/bash

# Calibrated Audio Feedback Validation System with Noise Floor Baseline
# Uses CMTECK microphone with proper noise floor calibration

TTS_URL="http://localhost:8091/speak"
SPEAKER="p254"
MIC_DEVICE="hw:4,0"
TEST_DIR="/tmp/calibrated_audio_validation"
LOG_FILE="$TEST_DIR/calibrated_results.log"
NOISE_FLOOR_FILE="$TEST_DIR/noise_floor.wav"

# Create test directory
mkdir -p $TEST_DIR

echo "=== CALIBRATED AUDIO VALIDATION STARTED $(date) ===" | tee $LOG_FILE

# Step 1: Establish noise floor baseline
establish_noise_floor() {
    echo "--- ESTABLISHING NOISE FLOOR BASELINE ---" | tee -a $LOG_FILE
    echo "Recording 10 seconds of ambient silence for noise floor measurement..." | tee -a $LOG_FILE
    
    # Record ambient silence
    arecord -D $MIC_DEVICE -f cd -t wav -d 10 $NOISE_FLOOR_FILE
    
    echo "Analyzing noise floor..." | tee -a $LOG_FILE
    
    # Analyze noise floor with sox
    if command -v sox > /dev/null; then
        local sox_output=$(sox "$NOISE_FLOOR_FILE" -n stat 2>&1)
        echo "Noise Floor Analysis:" | tee -a $LOG_FILE
        echo "$sox_output" | tee -a $LOG_FILE
        
        # Extract noise floor statistics
        local max_amplitude=$(echo "$sox_output" | grep "Maximum amplitude" | awk '{print $3}')
        local rms_amplitude=$(echo "$sox_output" | grep "RMS     amplitude" | awk '{print $3}')
        
        if [ ! -z "$max_amplitude" ] && [ ! -z "$rms_amplitude" ]; then
            NOISE_FLOOR_MAX_DB=$(echo "20 * l($max_amplitude) / l(10)" | bc -l)
            NOISE_FLOOR_RMS_DB=$(echo "20 * l($rms_amplitude) / l(10)" | bc -l)
            
            echo "Noise Floor Maximum: ${NOISE_FLOOR_MAX_DB} dB" | tee -a $LOG_FILE
            echo "Noise Floor RMS: ${NOISE_FLOOR_RMS_DB} dB" | tee -a $LOG_FILE
            
            # Set detection threshold 10dB above RMS noise floor
            DETECTION_THRESHOLD=$(echo "$NOISE_FLOOR_RMS_DB + 10" | bc -l)
            echo "Audio Detection Threshold: ${DETECTION_THRESHOLD} dB (RMS + 10dB)" | tee -a $LOG_FILE
        fi
    else
        echo "Sox not available, using default threshold" | tee -a $LOG_FILE
        DETECTION_THRESHOLD=-35.0
    fi
    
    echo "" | tee -a $LOG_FILE
}

# Function to perform calibrated microphone feedback test
calibrated_feedback_test() {
    local test_name="$1"
    local test_text="$2"
    local expected_duration="$3"
    
    echo "--- Testing: $test_name ---" | tee -a $LOG_FILE
    echo "Text: $test_text" | tee -a $LOG_FILE
    echo "Expected Duration: ${expected_duration}s" | tee -a $LOG_FILE
    echo "Using Detection Threshold: ${DETECTION_THRESHOLD} dB" | tee -a $LOG_FILE
    
    # Start microphone recording with buffer
    local mic_file="$TEST_DIR/test_$(date +%s).wav"
    local spinup_delay=2
    local recording_duration=$((expected_duration + 10))  # Buffer for TTS generation + audio
    
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
    
    # Analyze recorded audio with calibrated threshold
    analyze_calibrated_audio $mic_file $test_name
    
    # Check daemon logs for actual playback confirmation
    echo "Daemon Log Analysis:" | tee -a $LOG_FILE
    journalctl -u hal-tts.service --no-pager -n 10 --since "30 seconds ago" | grep -E "(SUCCESS|ERROR|PLAY|AUDIO)" | tail -3 | tee -a $LOG_FILE
    
    echo "" | tee -a $LOG_FILE
}

# Function to analyze recorded audio with calibrated noise floor
analyze_calibrated_audio() {
    local audio_file="$1"
    local test_name="$2"
    
    echo "Analyzing recorded audio: $audio_file" | tee -a $LOG_FILE
    
    # Check file size
    local file_size=$(stat -c%s "$audio_file")
    echo "Recording file size: $file_size bytes" | tee -a $LOG_FILE
    
    # Use sox for calibrated audio analysis
    if command -v sox > /dev/null; then
        echo "Performing calibrated SOX audio analysis..." | tee -a $LOG_FILE
        local sox_output=$(sox "$audio_file" -n stat 2>&1)
        echo "SOX Statistics:" | tee -a $LOG_FILE
        echo "$sox_output" | tee -a $LOG_FILE
        
        # Extract audio levels
        local max_amplitude=$(echo "$sox_output" | grep "Maximum amplitude" | awk '{print $3}')
        local rms_amplitude=$(echo "$sox_output" | grep "RMS     amplitude" | awk '{print $3}')
        
        if [ ! -z "$max_amplitude" ] && [ ! -z "$rms_amplitude" ]; then
            local max_db=$(echo "20 * l($max_amplitude) / l(10)" | bc -l)
            local rms_db=$(echo "20 * l($rms_amplitude) / l(10)" | bc -l)
            
            echo "Recording Peak Level: ${max_db} dB" | tee -a $LOG_FILE
            echo "Recording RMS Level: ${rms_db} dB" | tee -a $LOG_FILE
            echo "Noise Floor RMS: ${NOISE_FLOOR_RMS_DB} dB" | tee -a $LOG_FILE
            
            # Compare against calibrated threshold
            local rms_above_floor=$(echo "$rms_db - $NOISE_FLOOR_RMS_DB" | bc -l)
            echo "Signal above noise floor: ${rms_above_floor} dB" | tee -a $LOG_FILE
            
            # Audio detected if RMS is significantly above noise floor
            if (( $(echo "$rms_db > $DETECTION_THRESHOLD" | bc -l) )); then
                echo "✅ AUDIO DETECTED - Signal ${rms_above_floor}dB above noise floor" | tee -a $LOG_FILE
                return 0
            else
                echo "❌ NO SIGNIFICANT AUDIO - Signal only ${rms_above_floor}dB above noise floor" | tee -a $LOG_FILE
                return 1
            fi
        fi
    fi
    
    # Fallback analysis
    echo "❌ AUDIO ANALYSIS FAILED - Unable to perform calibrated measurement" | tee -a $LOG_FILE
    return 1
}

# Establish noise floor baseline first
establish_noise_floor

# Test 1: Simple audio test
calibrated_feedback_test "Basic Audio Test" "Testing audio output detection with calibrated noise floor measurement." 8

# Test 2: HAL pronunciation test
calibrated_feedback_test "HAL Pronunciation Test" "HAL should be pronounced as HAL, not H.A.L. Testing acronym pronunciation accuracy." 10

# Test 3: Sequential ordering test
calibrated_feedback_test "Sequential Ordering Test" "First sentence should play first. Second sentence should play second. Third sentence should play third." 15

# Performance and backend analysis
echo "--- SYSTEM PERFORMANCE ANALYSIS ---" | tee -a $LOG_FILE
kubectl get pods -n tts | tee -a $LOG_FILE

echo "--- BACKEND ENDPOINT PERFORMANCE ---" | tee -a $LOG_FILE
start_time=$(date +%s.%N)
curl -s -o /dev/null "http://localhost:5002/api/tts?text=performance%20test&speaker_id=p254"
end_time=$(date +%s.%N)
duration=$(echo "$end_time - $start_time" | bc -l)
echo "Load balancer endpoint response: ${duration}s" | tee -a $LOG_FILE

echo "=== CALIBRATED AUDIO VALIDATION COMPLETED $(date) ===" | tee -a $LOG_FILE
echo "Full results saved to: $LOG_FILE"
echo ""
echo "Audio recordings and noise floor baseline saved in: $TEST_DIR"
ls -la $TEST_DIR/*.wav 2>/dev/null | tee -a $LOG_FILE