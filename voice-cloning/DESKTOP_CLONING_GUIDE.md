# HAL Voice Cloning - Desktop Training Workflow

## Overview
Train HAL voice clone on powerful desktop PC, then export for production use.

## Phase 1: Desktop Setup (Your Desktop PC)

### 1. Install Coqui TTS with Training Support
```bash
# Install Coqui TTS with training capabilities
pip install TTS[all]

# Verify GPU support (if available)
python -c "import torch; print(f'CUDA: {torch.cuda.is_available()}')"
```

### 2. Copy Voice Samples to Desktop
Transfer the prepared HAL samples from this server:
```bash
# On desktop PC, copy from this server:
scp -r user@localhost:/home/user/dev/tts-pipeline/voice-cloning/hal-samples/ ./hal-training/
```

### 3. Desktop Voice Training Script
```python
#!/usr/bin/env python3
"""
HAL Voice Training Script for Desktop PC
Train custom XTTS voice using HAL samples
"""

import os
import torch
from TTS.tts.configs.xtts_config import XttsConfig
from TTS.tts.models.xtts import Xtts
from TTS.utils.generic_utils import get_user_data_dir
from pathlib import Path
import shutil

def train_hal_voice():
    # Setup paths
    samples_dir = Path("./hal-training/hal-samples")
    output_dir = Path("./hal-voice-model")
    
    # Create output directory
    output_dir.mkdir(exist_ok=True)
    
    print("🚀 Starting HAL 9000 Voice Training")
    
    # Load pre-trained XTTS model
    print("📦 Loading XTTS v2 base model...")
    config = XttsConfig()
    config.load_json("path/to/xtts/config.json")
    
    model = Xtts.init_from_config(config)
    
    # Fine-tune with HAL samples
    print("🎯 Fine-tuning with HAL voice samples...")
    # Training code here - use XTTS fine-tuning API
    
    # Export trained model
    print("💾 Exporting HAL voice model...")
    model.save_checkpoint(output_dir)
    
    print("✅ HAL voice training complete!")
    print(f"📁 Model saved to: {output_dir}")

if __name__ == "__main__":
    train_hal_voice()
```

## Phase 2: Production Integration (This Server)

### 1. Copy Trained Model to Production
```bash
# Transfer trained model from desktop to this server
scp -r desktop-pc:./hal-voice-model/ /home/user/dev/tts-pipeline/voice-cloning/
```

### 2. Replace VCTK Speaker p254
```bash
# Backup original p254
kubectl exec -n tts coqui-tts-pod -- cp -r /app/models/p254 /app/models/p254.backup

# Replace p254 with HAL voice
kubectl cp ./hal-voice-model/ tts-namespace/coqui-tts-pod:/app/models/p254/
```

### 3. Test Integration
```bash
# Test HAL voice with existing pipeline
curl -X POST "http://localhost:5002/api/tts" \
  -d "text=Good afternoon Dave. I am HAL 9000." \
  -d "speaker_id=p254" \
  --output test_hal_voice.wav
```

## Alternative: XTTS Direct Approach

### Desktop Training with XTTS CLI
```bash
# Use XTTS command-line for training
tts --model_name tts_models/multilingual/multi-dataset/xtts_v2 \
    --text "Good afternoon, Dave." \
    --speaker_wav ./hal-samples/hal_001.wav \
    --out_path hal_test.wav \
    --language_idx en

# Fine-tune speaker
tts-server --model_name tts_models/multilingual/multi-dataset/xtts_v2 \
           --use_cuda true
```

## Benefits of This Approach

1. **🖥️ Resource Separation**: Heavy training on desktop, lightweight production
2. **🔧 No System Impact**: Production TTS remains stable during training
3. **⚡ GPU Acceleration**: Desktop can use GPU for faster training
4. **🔄 Iterative**: Can retrain/adjust on desktop without affecting production
5. **📦 Portable**: Trained model is a simple file transfer

## File Checklist

### From This Server:
- ✅ HAL voice samples (10 WAV files, 22kHz mono)
- ✅ Original HAL MP3 collection (/home/user/hal-speech/)

### For Desktop Training:
- [ ] Coqui TTS with training support
- [ ] Training script
- [ ] HAL voice samples

### Production Deployment:
- [ ] Trained HAL voice model
- [ ] Integration script
- [ ] Backup of original p254
- [ ] Performance validation

## Next Actions

1. **Setup desktop environment** with Coqui TTS training capabilities
2. **Transfer voice samples** from this server to desktop
3. **Train HAL voice** on desktop PC with full resources
4. **Export and deploy** trained model to production
5. **Validate performance** and quality

This approach ensures zero impact on your production TTS system while leveraging your desktop's superior computational resources for voice training.