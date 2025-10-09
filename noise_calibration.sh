#!/bin/bash

# Background Noise Calibration for CMTech Conference Mic
# Measures ambient noise levels for proper VAD threshold setting

echo "🎤 CMTech Conference Mic - Background Noise Calibration"
echo "========================================================"
echo ""

# Mic configuration
MIC_DEVICE="hw:4,0"  # CMTECK device from arecord -l
SAMPLE_RATE=48000    # CMTECK supports 44100 or 48000 Hz
CHANNELS=2           # CMTECK is stereo only
DURATION=10          # 10 second calibration
OUTPUT_DIR="/tmp/audio_calibration"

# Create output directory
mkdir -p "$OUTPUT_DIR"

echo "📊 Test Configuration:"
echo "  Device: $MIC_DEVICE (CMTech Conference Mic)"
echo "  Sample Rate: ${SAMPLE_RATE}Hz"
echo "  Channels: $CHANNELS (stereo - will convert to mono for analysis)"
echo "  Duration: ${DURATION}s per test"
echo ""

# Test 1: Silent Environment (Machine Hum Only)
echo "🔇 TEST 1: SILENT ENVIRONMENT - BACKGROUND NOISE MEASUREMENT"
echo "------------------------------------------------------------"
echo "⚠️  Please remain COMPLETELY SILENT for ${DURATION} seconds"
echo "   This measures your machine's baseline hum and ambient noise"
echo ""
read -p "Press ENTER when ready to start silent measurement..."

echo "🎙️  Recording silent background for ${DURATION} seconds..."
arecord -D "$MIC_DEVICE" \
        -r "$SAMPLE_RATE" \
        -c "$CHANNELS" \
        -f S16_LE \
        -d "$DURATION" \
        "$OUTPUT_DIR/background_silent.wav" 2>/dev/null

if [ $? -eq 0 ]; then
    echo "✅ Silent background recorded successfully"
else
    echo "❌ Failed to record from CMTech mic"
    exit 1
fi

# Test 2: Typing/Keyboard Noise
echo ""
echo "⌨️  TEST 2: KEYBOARD NOISE MEASUREMENT"
echo "------------------------------------"
echo "Please type normally on your keyboard for ${DURATION} seconds"
echo "This measures typical working environment noise"
echo ""
read -p "Press ENTER when ready to start typing test..."

echo "🎙️  Recording keyboard noise for ${DURATION} seconds... START TYPING NOW!"
arecord -D "$MIC_DEVICE" \
        -r "$SAMPLE_RATE" \
        -c "$CHANNELS" \
        -f S16_LE \
        -d "$DURATION" \
        "$OUTPUT_DIR/background_typing.wav" 2>/dev/null

echo "✅ Typing noise recorded successfully"

# Test 3: Fan/Machine Noise Under Load
echo ""
echo "💻 TEST 3: MACHINE UNDER LOAD"
echo "-----------------------------"
echo "This will stress your CPU to measure fan noise under load"
echo ""
read -p "Press ENTER to start CPU stress test and audio recording..."

echo "🎙️  Recording machine noise under load for ${DURATION} seconds..."

# Start CPU stress in background
stress --cpu $(nproc) --timeout ${DURATION}s &
STRESS_PID=$!

# Record during stress test
arecord -D "$MIC_DEVICE" \
        -r "$SAMPLE_RATE" \
        -c "$CHANNELS" \
        -f S16_LE \
        -d "$DURATION" \
        "$OUTPUT_DIR/background_load.wav" 2>/dev/null

wait $STRESS_PID 2>/dev/null
echo "✅ Machine load noise recorded successfully"

# Analyze audio levels
echo ""
echo "📈 ANALYZING AUDIO LEVELS..."
echo "============================"

analyze_audio() {
    local file=$1
    local name=$2
    
    if [ -f "$file" ]; then
        echo ""
        echo "🔊 $name Analysis:"
        
        # Get peak and RMS levels using sox
        if command -v sox >/dev/null 2>&1; then
            sox "$file" -n stat 2>&1 | grep -E "(Maximum amplitude|RMS amplitude|Rough frequency)"
        else
            # Fallback using ffmpeg if sox not available
            if command -v ffmpeg >/dev/null 2>&1; then
                ffmpeg -i "$file" -af volumedetect -f null - 2>&1 | grep -E "(max_volume|mean_volume)"
            else
                echo "  ⚠️  Install sox or ffmpeg for detailed analysis"
                # Basic file size analysis
                size=$(stat -c%s "$file")
                echo "  File size: ${size} bytes"
            fi
        fi
        
        # Play a short sample for human verification
        echo "  🔊 Playing 2-second sample..."
        aplay "$file" &
        PLAY_PID=$!
        sleep 2
        kill $PLAY_PID 2>/dev/null
        wait $PLAY_PID 2>/dev/null
        
    else
        echo "❌ File not found: $file"
    fi
}

# Analyze each recording
analyze_audio "$OUTPUT_DIR/background_silent.wav" "Silent Background"
analyze_audio "$OUTPUT_DIR/background_typing.wav" "Keyboard Noise"
analyze_audio "$OUTPUT_DIR/background_load.wav" "Machine Under Load"

# Generate recommendations
echo ""
echo "🎯 VAD THRESHOLD RECOMMENDATIONS"
echo "==============================="
echo ""
echo "Based on your noise measurements:"
echo ""
echo "1. 🔇 Silent Background: Use as noise floor baseline"
echo "2. ⌨️  Keyboard Noise: Set VAD above this level to avoid false triggers"
echo "3. 💻 Machine Load: Maximum expected background noise"
echo ""
echo "Recommended VAD Configuration:"
echo "  - Noise Gate: Set 6dB above silent background level"
echo "  - Speech Detection: Set 10dB above keyboard noise level"
echo "  - Adaptive Threshold: Adjust based on current noise floor"
echo ""
echo "📁 Audio files saved to: $OUTPUT_DIR"
echo "   Use these for VAD calibration and testing"

# Clean up
echo ""
echo "✅ Noise calibration complete!"
echo "Next steps:"
echo "1. Install sox for detailed audio analysis: sudo apt install sox"
echo "2. Use these measurements to configure VAD thresholds"
echo "3. Test speech recognition with calibrated settings"