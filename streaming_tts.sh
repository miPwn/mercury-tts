#!/bin/bash

# Streaming parallel TTS with immediate playback
# Usage: ./streaming_tts.sh "sentence1" "sentence2" "sentence3" ...

TTS_URL="http://localhost:5002/api/tts"
SPEAKER="p227"
OUTPUT_DIR="/tmp/tts_streaming"
TIMESTAMP=$(date +%s%N)
COOKIE_JAR="/tmp/tts_cookies"

# Clean up
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# Pre-warm the TTS service
curl -X GET "${TTS_URL}?text=warm&speaker_id=${SPEAKER}" \
     -H "accept: audio/wav" \
     --output /dev/null \
     --silent \
     --cookie-jar "$COOKIE_JAR" \
     --keepalive-time 60 &

# Function to generate TTS for a single sentence
generate_tts() {
    local index=$1
    local text=$2
    local output_file="$OUTPUT_DIR/${TIMESTAMP}_${index}.wav"
    local ready_flag="$OUTPUT_DIR/${TIMESTAMP}_${index}.ready"
    
    curl -X GET "${TTS_URL}?text=${text}&speaker_id=${SPEAKER}" \
         -H "accept: audio/wav" \
         --output "$output_file" \
         --silent \
         --cookie "$COOKIE_JAR" \
         --keepalive-time 60
    
    # Signal that this chunk is ready
    touch "$ready_flag"
}

# Function to play chunks as they become ready
play_chunks() {
    local expected_index=1
    local max_index=$1
    
    while [[ $expected_index -le $max_index ]]; do
        local ready_flag="$OUTPUT_DIR/${TIMESTAMP}_${expected_index}.ready"
        local audio_file="$OUTPUT_DIR/${TIMESTAMP}_${expected_index}.wav"
        
        if [[ -f "$ready_flag" && -f "$audio_file" ]]; then
            echo "Streaming chunk $expected_index..."
            aplay "$audio_file" 2>/dev/null
            ((expected_index++))
        else
            sleep 0.1  # Brief wait before checking again
        fi
    done
}

# Launch all TTS requests in parallel
echo "Launching parallel TTS requests with streaming playback..."
index=1
total_chunks=$#

for sentence in "$@"; do
    # URL encode the sentence
    encoded_sentence=$(printf '%s' "$sentence" | sed 's/ /%20/g; s/,/%2C/g; s/\./%2E/g; s/!/%21/g; s/?/%3F/g; s/'"'"'/%27/g')
    generate_tts $index "$encoded_sentence" &
    ((index++))
done

# Start streaming playback immediately
play_chunks $total_chunks &
playback_pid=$!

# Wait for all generation to complete
wait

# Ensure playback finishes
wait $playback_pid

# Cleanup
rm -rf "$OUTPUT_DIR" "$COOKIE_JAR"