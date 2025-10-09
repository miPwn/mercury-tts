# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

The TTS Pipeline is a high-performance, low-latency Text-to-Speech system designed for real-time audio generation with sub-second response times. The system enables HAL 9000-style AI interactions and real-time conversation systems through sophisticated streaming audio delivery.

**Critical Performance Requirements** (Latency-Optimized):
- API Response Time: < 5ms (pre-warmed connections)
- Audio Latency: ~100-300ms (concurrent generation + network)
- Sequential Ordering: Race-condition-free chunk playback with parallel generation
- Concurrent Generation: 10+ parallel TTS requests across load-balanced endpoints
- Concurrent Capacity: 20+ requests per daemon

## Architecture Overview

### Core Components

**Primary Daemon** (Optimized):
- `streaming_safe_daemon.go` (Port 8091) - Latency-optimized concurrent generation with sequential coordination
- **Architecture**: Concurrent TTS generation across multiple endpoints with intelligent sequential playback
- **Status**: Sequential ordering issue RESOLVED and VALIDATED through predecessor-chain coordination

**Alternative Daemons**:
- `instant_tts_daemon.go` (Port 8091) - Zero-latency streaming with pre-warmed connections
- `tts_daemon_optimized.go` (Port 8081) - Advanced transport optimization with dual routing
- `fast_tts_daemon.go` (Port 8092) - High-throughput parallel processing

**Client Interface**:
- `speak_hal` - HAL 9000 voice client using p254 speaker (primary interface)
- `speak_client.go` - General command-line TTS interface
- `instant_speak.go` - Direct API client

**Backend Infrastructure** (Load-Balanced):
- **Coqui TTS Container**: `ghcr.io/coqui-ai/tts:latest` running in K3s cluster (3 healthy pods)
- **LoadBalancer Route**: `http://192.168.1.106:5002/api/tts` (primary endpoint - OPERATIONAL)
- **Service Discovery**: Automatic pod discovery through Kubernetes service
- **Load Balancing**: Requests distributed across healthy pods automatically
- **Voice Models**: 109 VCTK speakers (p225-p376), HAL uses p254
- **Performance**: 1.0s backend response time (improved from 22-52s)

## Common Development Commands

### Building and Testing
```bash
# Complete build with performance validation (RECOMMENDED)
./build.sh

# Manual component building
go build -o streaming_safe_daemon streaming_safe_daemon.go
go build -o speak_hal speak_client.go
go build -o instant_speak instant_speak.go
go build -o tts_test_client tts_test_client.go

# Development testing
./streaming_safe_daemon &  # Start primary daemon
./speak_hal "Good afternoon, Dave. This is HAL 9000."

# Health monitoring
curl http://localhost:8091/health
```

### Testing Commands
```bash
# Full test suite with performance validation
./build.sh

# Unit tests only
cd tests && go test -v -timeout=30s

# Performance benchmarks (critical for deployment)
cd tests && go test -bench=. -benchtime=5s -timeout=60s

# Specific critical tests
go test -run TestSequentialOrdering -timeout=30s
go test -run TestConcurrentRequestHandling -timeout=30s
go test -run TestLatencyRegression -timeout=30s

# Stress testing (non-CI only)
go test -run TestStressTest -timeout=120s
```

### LAN Access Commands
```bash
# Build remote client for other machines
go build -o remote_speak remote_speak.go

# Test LAN connectivity
./remote_speak --health

# Remote TTS from other WARP sessions
./remote_speak "Hello from remote WARP session"

# Health check from any LAN machine
curl http://192.168.1.106:8091/health

# Direct API call from other machines
curl -X POST http://192.168.1.106:8091/speak \
  -H "Content-Type: application/json" \
  -d '{"sentences":["Remote TTS test"], "speaker":"p254"}'
```

### Audio Validation Tools
```bash
# Comprehensive quality testing
./quality_test.sh

# Calibrated microphone feedback validation (RECOMMENDED)
./calibrated_audio_validator.sh

# Basic audio pipeline testing
go run audio_pipeline_test.go "Test sentence"

# End-to-end validation with noise floor calibration
./calibrated_audio_validator.sh
```

### Scaling Backend
```bash
# Scale Coqui TTS containers for higher throughput
kubectl scale deployment coqui-tts --replicas=3 -n tts
kubectl get pods -n tts

# Verify TTS server health
curl http://192.168.1.106:5002/health
```

## Critical Architecture Knowledge

### Sequential Ordering Issue (RESOLVED)
**Location**: `streaming_safe_daemon.go` lines 358-405 in `streamingPlaybackOptimized()` function
**Problem**: Audio chunks could play simultaneously instead of sequentially (FIXED)
**Solution Implemented**: Single-goroutine sequential processing with predecessor-chain coordination
**Validation**: Confirmed through calibrated microphone feedback testing and daemon logs
**Status**: ✅ OPERATIONAL - Sequential ordering guaranteed

### Performance Thresholds (BUILD BLOCKERS)
These thresholds are enforced by the build system and must not be exceeded:
- API Response: < 5ms
- First Audio: < 2s
- Concurrent Load: 10 requests
- Memory Usage: < 100MB per daemon
- Throughput: > 2.0 RPS minimum

### Non-Idempotent TTS Generation
**Issue**: Same input generates different audio files due to neural stochasticity
**Impact**: Cannot cache results reliably
**Mitigation**: Timestamp-based unique filenames, expect file differences

## Voice Profiles

**HAL 9000 Configuration**:
- **Speaker**: `p254` (deep, measured tone)
- **Usage**: All HAL interactions should use this profile
- **Client**: `./speak_hal` is pre-configured for p254

**Other Voices**:
- `p245`: Default balanced voice
- `p225`: Young female
- `p226`: Young male
- `p227`: Young female (clear)

## Configuration System

**Environment Configuration**:
The system uses `config.go` for centralized configuration management with environment variable overrides:

**Key Settings**:
- `TTS_LOADBALANCER_URL`: Primary TTS backend route
- `TTS_DIRECT_URL`: Direct cluster route (bypasses ingress)
- `DAEMON_PORT`: Local daemon port (default: 8091)
- `HAL_SPEAKER`: HAL voice profile (default: p254)
- `DEFAULT_SPEAKER`: Default voice profile (default: p245)

## Testing Strategy

**Comprehensive Testing**: See `docs/development/testing-strategy.md` for complete details.

**Critical Tests** (Must Pass):
1. **Sequential Ordering**: Primary failure mode detection
2. **Performance Benchmarks**: Latency threshold enforcement  
3. **Concurrent Handling**: Race condition prevention
4. **File Cleanup**: Memory leak prevention
5. **Health Monitoring**: System availability validation

**Test Environment Requirements**:
- Coqui TTS server running and healthy
- ALSA audio system (`aplay`) available
- Network connectivity to K3s cluster
- Go 1.19+ for compilation

## Network Architecture

**Data Flow** (LAN Accessible):
```
LAN Client → falcon.mipwn.local:8091 → Coqui TTS (5002) → Audio Hardware
```

**Remote Access**:
```
Other WARP Sessions → http://192.168.1.106:8091/speak → Centralized Audio Output
```

**Network Path**:
```
Starlink → Draytek Router → K3s Cluster → TTS Containers
```

**Connection Types**:
- Pre-warmed HTTP connections (5 per daemon)
- Connection pooling and reuse
- Dual routing (LoadBalancer + Direct) for optimization
- LAN accessibility on all interfaces (0.0.0.0:8091)

## Development Workflow

### Starting a Development Session
1. **Verify TTS Backend**: `kubectl get pods -n tts`
2. **Check System Health**: `curl http://192.168.1.106:5002/health`
3. **Build Components**: `./build.sh` (includes comprehensive testing)
4. **Start Primary Daemon**: `./streaming_safe_daemon &`
5. **Test Audio**: `./speak_hal "System operational check"`

### Making Changes
1. **Before Changes**: Run `./build.sh` to establish baseline
2. **After Changes**: Run `./build.sh` to validate (required)
3. **Critical**: Test sequential ordering manually by listening to multi-sentence requests
4. **Performance**: Ensure benchmarks pass before committing

### Debugging Sequential Ordering Issues
1. **Test Data**: Use 4+ identifiable sentences
2. **Manual Verification**: Listen carefully to playback order
3. **Concurrent Testing**: Run multiple requests simultaneously
4. **Audio Analysis**: Check `/tmp/streaming_safe_daemon/` for file generation timing

## Deployment Considerations

**Pre-Deployment Requirements**:
- All tests must pass via `./build.sh`
- Sequential ordering manually verified
- Performance benchmarks within thresholds
- TTS backend scaled for expected load
- Health endpoints responding correctly

**Production Scaling**:
- Scale Coqui TTS pods based on load: `kubectl scale deployment coqui-tts --replicas=N -n tts`
- Multiple daemon instances can run on different ports
- Monitor memory usage and connection pool utilization

## File Structure

**Source Files**:
- `streaming_safe_daemon.go` - Primary daemon (latency-optimized with LAN access)
- `instant_tts_daemon.go` - Zero-latency daemon (speed focus)  
- `tts_daemon_optimized.go` - Advanced optimization daemon
- `config.go` - Centralized configuration management
- `speak_client.go` - General TTS client
- `speak_hal` - HAL 9000 voice client (compiled)
- `remote_speak.go` - LAN client for other WARP sessions

**Testing**:
- `tests/daemon_test.go` - Unit and integration tests
- `tests/performance_test.go` - Benchmarks and regression tests
- `build.sh` - Automated build and test pipeline

**Documentation**:
- `docs/development/testing-strategy.md` - Comprehensive testing documentation
- `docs/architecture/` - System architecture details
- `PROJECT_RULE.md` - Detailed project context and current issues
- `WARP_TTS_RULE.md` - **CRITICAL**: Mandatory centralized TTS usage rule for all WARP sessions

## Documentation Maintenance Rule

**CRITICAL**: After any development session that modifies the codebase, documentation must be updated to reflect current state:

1. **Code Changes** → Update architecture docs and WARP.md
2. **API Changes** → Update client examples and endpoint documentation  
3. **New Components** → Update component listings and build commands
4. **Bug Fixes** → Update troubleshooting guides and known issues
5. **Performance Changes** → Update benchmark thresholds and performance targets

**Verification**: Documentation accuracy should be verified against actual codebase state, not assumed to be current.

This rule ensures future WARP instances have accurate, actionable guidance rather than outdated documentation that could lead to incorrect assumptions about system capabilities and architecture.

## Centralized TTS Service Rule

**MANDATORY**: All WARP sessions across the network MUST use the centralized TTS service at `http://192.168.1.106:8091` for audio commentary. See `WARP_TTS_RULE.md` for complete implementation details.

**Quick Usage**:
```bash
curl -X POST http://192.168.1.106:8091/speak \
  -H "Content-Type: application/json" \
  -d '{"sentences":["Your message"], "speaker":"p254"}'
```

This ensures unified voice output from falcon.mipwn.local across all WARP instances.
