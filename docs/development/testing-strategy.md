# Testing Strategy

## Overview

The TTS Pipeline employs a comprehensive multi-layered testing strategy designed to ensure reliability, performance, and correctness of the latency-optimized real-time audio generation system. Testing is critical due to the system's requirements for sub-second latency, concurrent generation across multiple TTS endpoints, sequential audio ordering, and race-condition-free operation.

## Testing Architecture

### Test Categories

#### 1. Unit Tests (`tests/daemon_test.go`)
**Purpose**: Validate individual component functionality and API contracts
**Location**: `tests/daemon_test.go`
**Execution**: `go test -v -timeout=30s ./tests/`

**Key Test Cases**:
- **Sequential Ordering Test** (`TestSequentialOrdering`)
  - **Architecture**: Tests concurrent generation with sequential playback coordination
  - **Validates**: Audio chunks play in correct order despite parallel TTS generation
  - **Manual Verification Required**: Human listening validation for proper sequence
  - **Test Data**: 4 identifiable sentences with sequential markers
  - **Concurrent Processing**: Verifies sentences generate in parallel across load-balanced endpoints

- **Concurrent Request Handling** (`TestConcurrentRequestHandling`)
  - **Validates**: System handles multiple simultaneous requests without race conditions
  - **Concurrency Level**: 5 parallel requests
  - **Success Criteria**: All requests complete successfully without interference

- **Health Endpoint Validation** (`TestHealthEndpoint`)
  - **Endpoint**: `GET /health`
  - **Required Fields**: `status`, `warmed_clients`, `audio_ready`, `timestamp`
  - **Validates**: System health monitoring and connection pool status

- **Error Handling** (`TestErrorHandling`)
  - **Invalid Requests**: Empty JSON, malformed JSON
  - **Expected Responses**: HTTP 400 for bad requests
  - **Validates**: Graceful degradation under error conditions

- **File Cleanup** (`TestFileCleanup`)
  - **Validates**: Temporary audio files are automatically cleaned up
  - **Directory**: `/tmp/streaming_safe_daemon/`
  - **Success Criteria**: No file accumulation after processing

- **Retry Mechanism** (`TestRetryMechanism`)
  - **Status**: Placeholder - requires mock TTS server implementation
  - **Purpose**: Validate exponential backoff and failure recovery

#### 2. Performance Tests (`tests/performance_test.go`)
**Purpose**: Enforce performance thresholds and prevent regressions
**Location**: `tests/performance_test.go`
**Execution**: `go test -bench=. -benchtime=5s -timeout=60s ./tests/`

**Performance Thresholds** (Latency-Optimized):
```go
MaxResponseLatency    = 5ms    // API response time (maintained)
MaxFirstAudioLatency  = 1s     // Improved: concurrent generation reduces latency
MaxConcurrentGeneration = 10   // Parallel TTS requests across endpoints
MaxConcurrentRequests = 20     // Increased capacity due to optimization
MaxMemoryUsage        = 100MB  // Memory limit per daemon (maintained)
```

**Benchmark Tests**:

- **API Response Time** (`BenchmarkAPIResponseTime`)
  - **Threshold**: < 5ms per request
  - **Validates**: Pre-warmed connection performance
  - **Failure Condition**: Latency exceeds threshold → build fails

- **Concurrent Load** (`BenchmarkConcurrentLoad`)
  - **Parallelism**: 10 concurrent workers
  - **Threshold**: < 10ms per request under load (2x normal threshold)
  - **Validates**: System performance under concurrent access

- **End-to-End Latency** (`BenchmarkEndToEndLatency`)
  - **Measures**: Complete pipeline latency (API → TTS → Audio playback)
  - **Threshold**: < 4s for complete processing
  - **Includes**: 3-second wait for audio processing completion

- **Memory Usage** (`BenchmarkMemoryUsage`)
  - **Monitors**: Memory allocation patterns
  - **Reports**: Allocations per request
  - **Validates**: No memory leaks in request processing

**Regression Tests**:

- **Latency Regression** (`TestLatencyRegression`)
  - **Baseline Measurements**:
    - Single sentence: < 2s
    - Multi-sentence (3): < 4s
    - Concurrent processing: < 3s
  - **Failure Condition**: Performance degrades beyond baseline

- **Throughput Test** (`TestThroughput`)
  - **Duration**: 5-second measurement window
  - **Minimum RPS**: 2.0 requests per second
  - **Validates**: System can maintain minimum throughput

- **Audio Quality** (`TestAudioQuality`)
  - **Generates**: Multiple samples of same input
  - **Purpose**: Consistency checking (manual verification)
  - **Note**: Non-idempotent TTS generation due to neural stochasticity

#### 3. Stress Tests (`TestStressTest`)
**Purpose**: Validate system behavior under high load conditions
**Execution**: Only in non-CI environments
**Skip Condition**: `testing.Short()` or `CI` environment variable set

**Stress Test Configuration**:
- **Concurrency**: 20 parallel workers
- **Requests per Worker**: 5 requests
- **Total Load**: 100 requests
- **Success Threshold**: 90% success rate
- **Request Spacing**: 100ms delay between requests per worker

#### 4. Integration Tests
**Purpose**: Validate complete system interactions
**Dependencies**: Requires running daemon instance

**Test Requirements**:
- Coqui TTS server availability (`http://192.168.1.106:5002/health`)
- Daemon running on port 8091
- ALSA audio system (`aplay` command available)
- `/tmp/streaming_safe_daemon/` directory access

## Build Integration

### Automated Testing (`build.sh`)

The build script enforces comprehensive testing before deployment:

```bash
# Test execution order
./build.sh
├── check_prerequisites()      # Verify Go, aplay, TTS server
├── build_components()         # Compile all daemons and clients
├── start_test_daemon()        # Launch streaming_safe_daemon
├── run_unit_tests()          # Execute unit test suite
├── run_performance_tests()   # Run benchmarks with thresholds
├── run_integration_tests()   # Test complete system integration
├── run_stress_tests()        # High-load validation (non-CI only)
└── generate_performance_report()  # Document test results
```

### Test Environment Setup

**Automated Daemon Startup**:
```bash
./streaming_safe_daemon > test_daemon.log 2>&1 &
TEST_DAEMON_PID=$!
sleep 5  # Wait for daemon initialization
curl -s http://localhost:8091/health  # Verify readiness
```

**Cleanup Process**:
- Automatic daemon termination
- Temporary file removal
- Connection pool cleanup
- Log file management

## Critical Test Cases

### Sequential Ordering (RESOLVED - CRITICAL VERIFICATION)
**Architecture**: Concurrent generation with predecessor-chain coordination
**Implementation**: `streamingPlaybackOptimized()` with intelligent sequential playback
**Test Validation**: 
- Generate 4 sequential sentences with identifiable content
- Verify concurrent generation across load-balanced TTS endpoints
- Manually verify audio plays in correct sequential order despite parallel processing
- **Success Criteria**: Sentences generate in parallel but play sequentially

### Race Condition Detection
**Purpose**: Ensure thread-safe audio playback
**Implementation**: Multiple concurrent requests with overlap detection
**Validation**: No interleaved audio streams

### Performance Regression Prevention
**Thresholds**: Enforce sub-5ms API response times
**Build Integration**: Performance test failures block deployment
**Monitoring**: Continuous latency measurement

## Test Data and Mocks

### Mock TTS Server
**Location**: `tests/daemon_test.go:createMockTTSServer()`
**Features**:
- Simulated 100ms TTS generation delay
- Returns mock WAV data
- HTTP 200 responses with proper headers

### Test Sentences
**Sequential Test Data**:
```
"First sentence should play first"
"Second sentence should play second"
"Third sentence should play third"  
"Fourth sentence should play fourth"
```

**Performance Test Data**:
- Single sentence: `"Performance test sentence"`
- Multi-sentence: 3 sentences with extended content
- HAL voice: Uses `p254` speaker profile

## Test Execution Commands

### Development Testing
```bash
# Full test suite
./build.sh

# Unit tests only
cd tests && go test -v -timeout=30s

# Performance benchmarks
cd tests && go test -bench=. -benchtime=5s -timeout=60s

# Specific test
go test -run TestSequentialOrdering -timeout=30s
```

### Manual Testing
```bash
# Start daemon
./streaming_safe_daemon &

# Test HAL voice
./speak_hal "System operational check"

# Health check
curl http://localhost:8091/health

# Load test
./tts_test_client
```

## Monitoring and Validation

### Performance Metrics
- API response latency distribution
- Memory allocation patterns
- Connection pool utilization
- Audio generation timing
- File cleanup verification

### Health Indicators
- Daemon health endpoint status
- TTS server connectivity
- Audio system availability
- Temporary file accumulation

### Failure Detection
- Performance threshold violations
- Sequential ordering failures
- Memory leak detection
- Connection pool exhaustion
- Audio system failures

### Test Environment Requirements

### System Dependencies
- Go 1.19+ compiler
- ALSA audio system (`aplay`)
- Multiple Coqui TTS containers (K3s cluster) for load balancing
- Network connectivity to both primary and direct TTS endpoints
- Writable `/tmp/streaming_safe_daemon/` directory
- Load balancing across `ttsURL` and `ttsDirectURL` endpoints

### Performance Environment
- Stable network connection
- Dedicated testing resources
- No competing audio processes
- Sufficient memory (>200MB available)

## Continuous Integration

### CI Pipeline Integration
- Skip stress tests in CI (`CI` environment variable)
- Enforce performance thresholds
- Require health check passes
- Validate build artifact creation

### Success Criteria
- All unit tests pass
- Performance benchmarks within thresholds
- Integration tests complete successfully
- No memory leaks detected
- Sequential ordering verified

This testing strategy ensures the latency-optimized TTS Pipeline maintains its critical performance characteristics with concurrent generation across multiple TTS endpoints while guaranteeing sequential audio ordering for optimal real-time conversation quality.
