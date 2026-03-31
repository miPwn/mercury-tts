# Voice Architecture

## CRITICAL: DO NOT MODIFY WITHOUT READING THIS DOCUMENT

This system uses **two completely independent voice pipelines**. Changing one MUST NOT affect the other.

## Architecture Overview

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────────────┐
│ hal command  │────>│ hal-tts daemon   │────>│ coqui-tts (VCTK)    │
│ (streaming)  │     │ port 8091        │     │ port 5002           │
│ speaker:p254 │     │ 2 replicas       │     │ 1 replica           │
└─────────────┘     └──────────────────┘     └─────────────────────┘

┌─────────────┐     ┌─────────────────────────────────────────────┐
│ halo command │────>│ coqui-xtts (XTTS v2 voice cloning)         │
│ (cloned)     │     │ port 5003                                   │
│ HAL-006 ref  │     │ 1 replica, patched server.py at startup     │
└─────────────┘     └─────────────────────────────────────────────┘
```

## Services (all in k8s namespace: tts)

### hal-tts (port 8091) — DO NOT TOUCH
- Go streaming daemon (`streaming_safe_daemon`)
- 2 replicas, LoadBalancer on 192.168.1.106:8091
- Receives JSON `{"sentences":["..."],"speaker":"p254"}`
- Calls coqui-tts VCTK at port 5002 for audio generation
- Streams audio to system speakers via PulseAudio
- Used by: `hal` command, hal-sysmon alerts

### coqui-tts (port 5002) — DO NOT TOUCH
- Stock Coqui TTS image: `ghcr.io/coqui-ai/tts:latest`
- Model: `tts_models/en/vctk/vits` (multi-speaker, fast)
- Valid speakers: p225-p376, ED
- Used by: hal-tts daemon only

### coqui-xtts (port 5003) — Voice Cloning
- Stock Coqui TTS image with startup patch for XTTS voice cloning
- Model: `tts_models/multilingual/multi-dataset/xtts_v2` (voice cloning)
- Reference WAV: `/app/references/001.wav` (HAL-006 cloned voice)
- Startup patch fixes Coqui 0.22.0 server.py bug (style_wav -> speaker_wav mapping)
- Uses shared PVC `tts-models` (same as coqui-tts)
- Memory: 3Gi request, 6Gi limit (XTTS model is ~1.8GB)
- Used by: `halo` command only

## Commands

### hal (fast streaming voice)
- Location: `/usr/local/bin/hal` -> symlink to this repo's `hal` script
- Speaker: `p254` (VCTK)
- Supports multiple sentences: `hal "sentence 1" "sentence 2"`
- Pipeline: hal -> hal-tts (8091) -> coqui-tts (5002) -> speakers

### halo (HAL-006 cloned voice)
- Location: `/usr/local/bin/halo` -> symlink to this repo's `halo` script
- Voice: HAL-006 via XTTS v2 voice cloning with reference WAV
- Single sentence only: `halo "dramatic statement"`
- Pipeline: halo -> coqui-xtts (5003/XTTS) -> downloads WAV -> aplay
- Slower (5-15s) but authentic cloned voice

## Shared Resources

- **PVC `tts-models`** (20Gi, RWO): Shared between coqui-tts and coqui-xtts
  - Both pods run on same node (falcon), so RWO works for both
  - Contains downloaded model files for VCTK, XTTS v2, LJSpeech
- **ConfigMap `hal-voice-reference`**: Contains 001.wav HAL-006 reference audio
  - Mounted at `/app/references/` in both Coqui deployments
- **PulseAudio socket**: Mounted in hal-tts for direct audio playback

## What NOT to change

1. **coqui-tts deployment** — Changing the model or args breaks ALL hal voice output
2. **hal-tts deployment** — Changing this breaks the streaming daemon
3. **hal script** — The speaker `p254` and endpoint `8091` are hardcoded for stability
4. **PVC tts-models** — Both Coqui instances depend on this
5. **ConfigMap hal-voice-reference** — Both mount this for the HAL-006 WAV

## Troubleshooting

### hal voice not working
```bash
kubectl get pods -n tts -l app=hal-tts
curl http://localhost:8091/health
kubectl logs -n tts deployment/hal-tts
```

### halo voice not working
```bash
kubectl get pods -n tts -l app.kubernetes.io/instance=coqui-xtts
curl http://localhost:5003/
kubectl logs -n tts deployment/coqui-xtts
```

### XTTS pod crash-looping
- Check memory: XTTS needs 3-6GB RAM
- Check model files: `kubectl exec -n tts deployment/coqui-tts -- ls /root/.local/share/tts/tts_models--multilingual--multi-dataset--xtts_v2/`
- Startup takes 60-90s for model load — readiness probe has 90s initial delay
