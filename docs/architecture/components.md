# Component Architecture

## System Components Overview

The TTS Pipeline consists of several interconnected components, each serving a specific role in the audio generation and playback process.

## Core Components

### 1. Coqui TTS Container (Backend)

**Container Image**: `ghcr.io/coqui-ai/tts:latest`
**Deployment**: K3s cluster
**Port**: 5002
**Model**: `tts_models/en/vctk/vits`

#### Container Configuration
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: coqui-tts
spec:
  replicas: 1
  selector:
    matchLabels:
      app: coqui-tts
  template:
    metadata:
      labels:
        app: coqui-tts
    spec:
      containers:
      - name: tts-server
        image: ghcr.io/coqui-ai/tts:latest
        command: ["tts-server"]
        args: 
          - "--model_name"
          - "tts_models/en/vctk/vits"
          - "--port"
          - "5002"
        ports:
        - containerPort: 5002
        resources:
          requests:
            memory: "2Gi"
            cpu: "1000m"
          limits:
            memory: "4Gi"
            cpu: "2000m"
```

#### Network Access
- **LoadBalancer Route**: `http://localhost:5002/api/tts`
- **Direct Cluster Route**: `http://10.43.19.35:5002/api/tts`
- **Health Check**: `http://localhost:5002/health`

#### Supported Voice Models
- **VCTK Dataset**: 109 different speakers (p225-p376)
- **Popular Voices**:
  - `p245` - Default balanced voice
  - `p254` - HAL 9000 style (deep, measured)
  - `p225` - Young female
  - `p226` - Young male
  - `p227` - Young female (clear)

### 2. TTS Daemon Layer (Middleware)

#### Instant TTS Daemon (Recommended)
**File**: `instant_tts_daemon.go`
**Port**: 8091
**Purpose**: Zero-latency streaming with pre-warmed connections

**Features**:
- Pre-warmed HTTP connection pool (5 connections)
- Pre-initialized audio system
- Streaming playback as chunks arrive
- Automatic cleanup of temporary files
- Drag detection (>3s chunks)

#### Optimized TTS Daemon
**File**: `tts_daemon_optimized.go`
**Port**: 8081
**Purpose**: Advanced transport optimization with dual routing

**Features**:
- Dual routing (LoadBalancer + Direct)
- Advanced HTTP transport optimization
- Performance benchmarking endpoints
- Detailed timing metrics
- Connection pooling and reuse

#### Pipeline TTS Daemon
**File**: `pipeline_tts_daemon.go`
**Port**: 8090
**Purpose**: Sequential processing with controlled concurrency

**Features**:
- Sequential chunk playback
- Controlled concurrency limits
- Pipeline processing architecture
- Memory-efficient streaming

#### Fast TTS Daemon
**File**: `fast_tts_daemon.go`
**Port**: 8092
**Purpose**: High-throughput parallel processing

**Features**:
- Maximum concurrent processing
- Parallel chunk generation
- High-throughput optimization
- Batch processing capabilities

### 3. Client Layer (Frontend)

#### Speak Client
**File**: `speak_client.go`
**Purpose**: Command-line interface for TTS requests

**Usage**:
```bash
./speak_hal "Good afternoon, Dave" "This is HAL 9000"
```

#### Instant Speak Client
**File**: `instant_speak.go`
**Purpose**: Direct API client for instant daemon

**Features**:
- JSON request formatting
- Response timing measurement
- Error handling and reporting

#### Test Client
**File**: `tts_test_client.go`
**Purpose**: Performance testing and benchmarking

**Features**:
- Load testing capabilities
- Performance metrics collection
- Comparative benchmarking
- Stress testing tools

### 4. Audio Playback Engine

#### ALSA Integration
**Command**: `aplay`
**Format**: WAV, 22kHz, 16-bit, PCM
**Buffer**: Minimal buffering for low latency

#### Audio Pipeline
```
TTS Container → HTTP Stream → Local File → aplay → Audio Device
```

## Component Interactions

### Request Flow
```
Client Application
    ↓ POST /speak (JSON)
TTS Daemon
    ↓ GET /api/tts?text=...&speaker_id=p254
Coqui TTS Container
    ↓ WAV Audio Stream
TTS Daemon (temp file)
    ↓ aplay command
Audio Hardware
```

### Health Monitoring Flow
```
TTS Daemon /health endpoint
    ↓ HTTP GET health check
Coqui TTS Container /health
    ↓ Response: healthy/unhealthy
TTS Daemon aggregated health status
    ↓ JSON response
Client/Monitoring System
```

## Configuration Management

### Environment Variables
- `TTS_URL`: Coqui TTS server endpoint
- `TTS_PORT`: Local daemon port
- `DEFAULT_SPEAKER`: Default voice profile
- `AUDIO_DEVICE`: ALSA audio device

### Configuration Files
- `daemon.conf`: Daemon-specific settings
- `audio.conf`: Audio system configuration
- `cluster.conf`: K3s cluster settings

## Resource Requirements

### Coqui TTS Container
- **CPU**: 1-2 cores (inference)
- **Memory**: 2-4GB (model loading)
- **Storage**: 1GB (model cache)
- **Network**: 100Mbps (audio streaming)

### TTS Daemons
- **CPU**: 0.1-0.5 cores (HTTP processing)
- **Memory**: 50-100MB (connection pools)
- **Storage**: 10MB (temporary files)
- **Network**: 50Mbps (proxy traffic)

### Client Applications
- **CPU**: Minimal (HTTP requests)
- **Memory**: <10MB per instance
- **Network**: 10Mbps (request/response)

## Scalability Considerations

### Horizontal Scaling
- Multiple daemon instances behind load balancer
- Coqui TTS container replicas in cluster
- Client connection pooling and distribution

### Vertical Scaling
- Increase daemon connection pool sizes
- Add CPU/memory to TTS container
- Optimize audio buffer sizes

## Fault Tolerance

### Component Failures
- **TTS Container Down**: Daemon health checks fail, clients receive errors
- **Daemon Down**: Clients fail to connect, automatic retry mechanisms
- **Audio System Failure**: Silent operation, error logging

### Recovery Mechanisms
- Automatic container restart (K3s)
- Connection pool refresh (daemons)
- Client retry logic with exponential backoff