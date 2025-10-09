# System Overview

## Architecture Summary

The TTS Pipeline is a distributed, high-performance text-to-speech system designed for real-time audio generation with minimal latency. The system consists of multiple daemon implementations, each optimized for specific use cases.

## Core Components

### 1. TTS Daemons (Servers)
- **Instant TTS Daemon** - Zero-latency streaming with pre-warmed connections
- **Optimized TTS Daemon** - Advanced transport optimization with dual routing
- **Pipeline TTS Daemon** - Sequential processing with controlled concurrency
- **Fast TTS Daemon** - High-throughput parallel processing

### 2. Client Libraries
- **Speak Client** - Simple command-line interface for TTS requests
- **Instant Speak** - Direct API client for instant daemon
- **Test Client** - Performance testing and benchmarking

### 3. External Dependencies
- **Coqui TTS Server** - Neural TTS model inference (port 5002)
- **ALSA Audio System** - Audio playback via `aplay`
- **K3s Cluster** - Container orchestration for TTS server

## System Flow

```
[Client Application] 
    ↓ HTTP POST /speak
[TTS Daemon] 
    ↓ HTTP GET /api/tts
[Coqui TTS Server]
    ↓ WAV Audio Stream
[Audio Playback Engine]
    ↓ PCM Audio
[Audio Hardware]
```

## Key Design Principles

### 1. **Minimal Latency**
- Pre-warmed HTTP connections
- Instant audio playback as chunks arrive
- Zero blocking on audio generation

### 2. **High Throughput**
- Concurrent TTS request processing
- Connection pooling and reuse
- Streaming audio delivery

### 3. **Fault Tolerance**
- Graceful error handling
- Automatic retry mechanisms
- Health monitoring endpoints

### 4. **Scalability**
- Multiple daemon implementations
- Load balancing across TTS backends
- Configurable concurrency limits

## Performance Characteristics

- **Response Time**: < 5ms for pre-warmed connections
- **Audio Latency**: ~200-500ms (TTS generation + network)
- **Throughput**: 10+ concurrent requests per daemon
- **Memory Usage**: ~50MB per daemon (with connection pools)

## Network Architecture

```
Internet → Starlink → Draytek Router → K3s Cluster
                           ↓
                    Local TTS Daemon (8091)
                           ↓
                    Audio Playback (ALSA)
```

### Network Paths
1. **LoadBalancer Route**: Client → Daemon → 192.168.1.106:5002 → Coqui TTS
2. **Direct Route**: Client → Daemon → 10.43.19.35:5002 → Coqui TTS (bypasses ingress)

## Deployment Topology

### Development Environment
- Single daemon instance
- Local Coqui TTS container
- Direct audio device access

### Production Environment
- Multiple daemon instances (load balanced)
- Clustered Coqui TTS backend
- Container-based audio processing

## Security Considerations

### Network Security
- Internal cluster communication only
- No external TTS API exposure
- Local audio device access required

### Data Security
- No persistent storage of audio content
- Automatic cleanup of temporary files
- In-memory audio processing

## Monitoring and Observability

### Health Endpoints
- `/health` - System status and connectivity
- `/stats` - Performance metrics and statistics
- `/benchmark` - Comparative performance testing

### Metrics Tracked
- Request latency and throughput
- Connection pool utilization
- Audio generation and playback timing
- Error rates and retry counts

## Integration Points

### Input Interfaces
- HTTP POST requests with JSON payloads
- Command-line client tools
- Shell script integration

### Output Interfaces
- ALSA audio playback
- HTTP response with performance metrics
- Log-based monitoring and debugging

This architecture enables real-time conversation systems, voice assistants, and batch audio processing with optimal performance characteristics.