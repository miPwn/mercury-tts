#!/bin/bash

# Parallel TTS processing with ordered playback
# Usage: ./parallel_tts.sh "sentence1" "sentence2" "sentence3" ...

TTS_URL="http://192.168.1.106:5002/api/tts"
SPEAKER="p227"
OUTPUT_DIR="/tmp/tts_chunks"
TIMESTAMP=$(date +%s%N)

# Clean up any existing chunks
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# Function to generate TTS for a single sentence
generate_tts() {
    local index=$1
    local text=$2
    local output_file="$OUTPUT_DIR/${TIMESTAMP}_${index}.wav"
    
    curl -X GET "${TTS_URL}?text=${text}&speaker_id=${SPEAKER}" \
         -H "accept: audio/wav" \
         --output "$output_file" \
         --silent
    
    echo "Generated: $output_file"
}

# Launch all TTS requests in parallel
echo "Launching parallel TTS requests..."
index=1
for sentence in "$@"; do
    # URL encode the sentence
    encoded_sentence=$(printf '%s' "$sentence" | sed 's/ /%20/g; s/,/%2C/g; s/\./%2E/g; s/!/%21/g; s/?/%3F/g; s/'"'"'/%27/g')
    generate_tts $index "$encoded_sentence" &
    ((index++))
done

# Wait for all background processes to complete
wait

# Play files in order
echo "Playing audio chunks in sequence..."
for file in $(ls "$OUTPUT_DIR"/${TIMESTAMP}_*.wav | sort -V); do
    if [[ -f "$file" ]]; then
        echo "Playing: $file"
        aplay "$file" 2>/dev/null
    fi
done

# Cleanup
rm -rf "$OUTPUT_DIR"