# TTS Pipeline Project Rule

## Project Overview
This is a high-performance, low-latency Text-to-Speech (TTS) pipeline system located at `/home/mipwn/dev/tts-pipeline`. The system provides real-time audio generation with minimal latency for voice assistants, conversation systems, and HAL 9000-style AI interactions.

## System Architecture

### Core Components
1. **Coqui TTS Container Backend**: Neural TTS server running in K3s cluster
   - Image: `ghcr.io/coqui-ai/tts:latest`
   - Model: `tts_models/en/vctk/vits`
   - LoadBalancer Route: `http://192.168.1.106:5002/api/tts`
   - Direct Route: `http://10.43.19.35:5002/api/tts`
   - Supports 109 VCTK speaker voices (p225-p376)

2. **TTS Daemon Layer (Middleware)**:
   - **instant_tts_daemon.go** (Port 8091) - Zero-latency streaming with pre-warmed connections
   - **streaming_safe_daemon.go** (Port 8091) - Race-condition-safe sequential playback
   - **tts_daemon_optimized.go** (Port 8081) - Advanced transport optimization
   - **fast_tts_daemon.go** (Port 8092) - High-throughput parallel processing

3. **Client Layer**:
   - **speak_client.go** - Command-line interface for TTS requests
   - **speak_hal** - HAL 9000 voice client using p254 speaker
   - **instant_speak.go** - Direct API client for instant daemon

### Network Infrastructure
- **Starlink Internet** → **Draytek 2927ax Router** → **K3s Cluster**
- **Multiple Coqui TTS pods** for load distribution (scaled via kubectl)
- **Pre-warmed HTTP connections** and **audio system initialization**

## Key Features & Performance
- **API Response Time**: < 5ms for pre-warmed connections
- **Audio Latency**: ~200-500ms (TTS generation + network)
- **Concurrent Processing**: 10+ requests per daemon with throttling
- **Sequential Ordering**: Race-condition-free chunk playback
- **Automatic Retry**: Exponential backoff for failed requests
- **File Cleanup**: Automatic temporary file removal after playback

## Voice Profiles
- **p245**: Default balanced voice
- **p254**: HAL 9000 style (deep, measured) - preferred for AI assistants
- **p225-p376**: Full VCTK dataset with various speaker characteristics

## Critical Performance Requirements
1. **Zero Race Conditions**: Audio chunks must play in strict sequential order
2. **Minimal Latency**: Start playback immediately when first chunk is ready
3. **No Throttling Delays**: Process requests concurrently without artificial blocking
4. **Pre-warmed Systems**: HTTP connections and audio subsystem ready instantly

## Current Issues & Solutions
### Sequential Ordering Bug (CRITICAL)
- **Problem**: streamingPlayback() plays multiple chunks simultaneously
- **Location**: streaming_safe_daemon.go lines 283-300
- **Impact**: Audio plays out of order (e.g., sentence 3 before sentence 2)
- **Solution**: Implement proper sequential playback with audio completion synchronization

### Non-Idempotent TTS
- **Issue**: Same input generates different audio files (neural stochasticity)
- **Impact**: Cannot cache results reliably
- **Mitigation**: Use timestamp-based unique filenames

## Development Workflow

### Building & Testing
```bash
# Run comprehensive build with performance validation
./build.sh

# Manual daemon testing
./streaming_safe_daemon &  # Start daemon
./speak_hal "Test message"  # Test HAL voice

# Performance benchmarks
cd tests && go test -bench=. -timeout=60s
```

### Scaling TTS Backend
```bash
# Scale Coqui TTS containers for higher throughput
kubectl scale deployment coqui-tts --replicas=3 -n tts
kubectl get pods -n tts  # Verify scaling
```

### Monitoring & Health
- Health endpoint: `curl http://localhost:8091/health`
- Performance stats: Check daemon logs for timing metrics
- Audio verification: Listen for sequential ordering in multi-sentence requests

## File Structure
```
/home/mipwn/dev/tts-pipeline/
├── docs/                     # Comprehensive documentation
├── tests/                    # Unit, performance, and integration tests
├── *.go                     # Source files for daemons and clients
├── build.sh                 # Automated build and test script
├── README.md                # Project overview
└── .gitignore              # Git exclusion patterns
```

## Performance Optimization Targets
1. **API Response**: Maintain < 5ms response time
2. **First Audio**: Target < 500ms to first chunk playback
3. **Memory Usage**: Keep daemon memory < 100MB
4. **Concurrent Capacity**: Support 20+ simultaneous requests
5. **Audio Quality**: Ensure consistent neural TTS generation

## Integration Points
- **Input**: HTTP POST /speak with JSON payload {"sentences": [], "speaker": "p254"}
- **Output**: ALSA audio playback via `aplay` command
- **Monitoring**: JSON health responses with connection pool status
- **Dependencies**: Requires Coqui TTS container availability

## Session Handoff Instructions
When working on this project:
1. **Check TTS container status**: `kubectl get pods -n tts`
2. **Verify daemon health**: `curl http://localhost:8091/health`
3. **Test audio output**: `./speak_hal "System operational check"`
4. **Monitor sequential ordering**: Listen carefully to multi-sentence requests
5. **Run performance tests**: `./build.sh` before any deployments

## Critical Knowledge
- **Never modify without testing sequential ordering** - this is the primary failure mode
- **Always scale TTS pods under load** - single pod becomes bottleneck
- **Performance tests must pass** - build.sh enforces latency thresholds
- **HAL voice uses p254** - specifically tuned for AI assistant interactions

This system enables real-time conversation with sub-second latency and race-condition-free sequential audio playback.