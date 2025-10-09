# TTS Pipeline

A high-performance, low-latency Text-to-Speech pipeline for real-time audio generation and streaming playback.

## Overview

This project provides multiple TTS daemon implementations optimized for different use cases:
- **Instant TTS Daemon**: Zero-latency streaming with pre-warmed connections
- **Optimized TTS Daemon**: Advanced connection pooling and route optimization
- **Pipeline TTS Daemon**: Sequential processing with controlled concurrency
- **Fast TTS Streamer**: Parallel processing for maximum throughput

## Features

- **Pre-warmed HTTP connections** for instant response
- **Streaming audio playback** - play chunks as they become available
- **Multiple voice profiles** (p245, p254, etc.) via Coqui TTS
- **Drag detection** - identifies chunks taking longer than 3 seconds
- **Automatic cleanup** - removes temporary audio files after playback
- **Health monitoring** - endpoint status and performance metrics
- **Concurrent processing** with controlled request flow

## Quick Start

### Prerequisites

- Go 1.19+ installed
- `aplay` (ALSA) for audio playback
- Coqui TTS server running on port 5002

### Running the System

1. **Start the Instant TTS Daemon:**
   ```bash
   go build -o instant_tts_daemon instant_tts_daemon.go
   ./instant_tts_daemon
   ```

2. **Use the HAL Voice Client:**
   ```bash
   go build -o speak_hal speak_client.go
   ./speak_hal "Good afternoon, Dave. This is HAL 9000."
   ```

## Architecture

### Instant TTS Daemon (Recommended)
- **Port**: 8091
- **Endpoints**: `/speak`, `/health`
- **Features**: Pre-warmed connections, instant audio playback
- **Use Case**: Real-time conversation, minimal latency

### Optimized TTS Daemon
- **Port**: 8081  
- **Endpoints**: `/tts`, `/play`, `/stats`, `/health`, `/benchmark`
- **Features**: Advanced transport optimization, dual routing
- **Use Case**: High-throughput batch processing

## Voice Profiles

- **p245**: Default balanced voice
- **p254**: HAL 9000 style voice (recommended for AI assistants)
- **p{xxx}**: Various VCTK speaker profiles

## Performance

- **Response time**: < 5ms for pre-warmed connections
- **Audio playback**: Starts immediately when first chunk is ready
- **Concurrent requests**: Handled with intelligent throttling
- **Memory usage**: Optimized with connection pooling

## Configuration

Key constants in each daemon:
- `ttsURL`: Coqui TTS server endpoint
- `serverPort`: Local daemon port
- `defaultSpeaker`: Default voice profile
- `dragThreshold`: Performance monitoring threshold

## Development

### Adding New Voice Profiles
Modify the speaker parameter in client calls or daemon defaults.

### Performance Tuning
Adjust connection pool sizes, timeouts, and concurrent limits based on your hardware.

### Monitoring
Use the `/health` and `/stats` endpoints to monitor system performance.

## Troubleshooting

1. **No audio output**: Check `aplay` installation and audio device configuration
2. **Connection refused**: Verify TTS daemon is running on expected port
3. **Slow performance**: Check network connectivity to Coqui TTS server
4. **Memory issues**: Adjust connection pool sizes in daemon configuration

## License

MIT License - See LICENSE file for details.