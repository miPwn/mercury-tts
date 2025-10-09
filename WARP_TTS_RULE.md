# WARP Centralized TTS Service Rule

## Overview
All WARP sessions across the mipwn.com network infrastructure MUST use the centralized TTS service at `falcon.mipwn.local:8091` for audio commentary and voice interface. This ensures unified voice output from a single, optimized audio system.

## Rule Implementation

### CRITICAL: All WARP Sessions Must Use Centralized TTS

**Service Location**: 
- **Primary**: `http://192.168.1.106:8091` 
- **DNS**: `http://falcon.mipwn.local:8091` (when available)
- **Audio Output**: Plays through falcon.mipwn.local speakers only

### Voice Commentary Requirements

All WARP instances MUST use this service for:
1. **Status Updates**: System notifications and progress reports
2. **Error Messages**: Critical alerts and diagnostic information  
3. **Completion Notifications**: Task completion confirmations
4. **HAL 9000 Persona**: All HAL-style commentary and responses
5. **Debug Information**: Technical details requiring audio output

### API Usage Protocol

**Standard Request Format**:
```bash
curl -X POST http://192.168.1.106:8091/speak \
  -H "Content-Type: application/json" \
  -d '{"sentences":["Your message here"], "speaker":"p254"}'
```

**Health Check Before Use**:
```bash
curl -s http://192.168.1.106:8091/health
```

**Expected Response**:
```json
{
  "status": "streaming-safe-ready",
  "warmed_clients": 5,
  "audio_ready": true,
  "timestamp": 1760023777
}
```

### Implementation Guidelines

#### For WARP Sessions on Remote Machines

1. **Test Connectivity First**:
   ```bash
   curl -s http://192.168.1.106:8091/health || echo "TTS service unavailable"
   ```

2. **Use HAL Voice Profile**:
   - Always specify `"speaker":"p254"` for HAL 9000 consistency
   - This ensures consistent voice characteristics across all sessions

3. **Handle Network Failures Gracefully**:
   - If TTS service is unavailable, continue operation silently
   - Log the audio request for debugging but don't block execution

4. **Sentence Formatting**:
   - Keep sentences concise and clear
   - Use proper punctuation for natural speech patterns
   - Break long messages into multiple sentences

#### Example WARP Session Integration

```go
func speakViaService(message string) {
    payload := map[string]interface{}{
        "sentences": []string{message},
        "speaker":   "p254",
    }
    
    jsonData, _ := json.Marshal(payload)
    
    _, err := http.Post("http://192.168.1.106:8091/speak", 
        "application/json", bytes.NewBuffer(jsonData))
    
    if err != nil {
        log.Printf("TTS service unavailable: %v", err)
        // Continue operation silently
    }
}
```

### Voice Interface Standards

**HAL 9000 Persona Requirements**:
- Use measured, calm delivery
- Technical precision in language
- Address user as "Dave" when appropriate
- Maintain authoritative but helpful tone

**Message Categories**:

1. **System Status**: 
   - "All systems operational, Dave."
   - "Network connectivity established."
   - "Processing request with optimal efficiency."

2. **Task Progress**:
   - "Analysis in progress. Estimated completion in X minutes."
   - "Data processing at 85% completion."
   - "Mission objectives achieved successfully."

3. **Error Handling**:
   - "I'm detecting an anomaly in the system, Dave."
   - "Network connectivity has been compromised."
   - "Unable to complete the requested operation due to insufficient resources."

4. **Completion Notifications**:
   - "Task completed with computational precision."
   - "All objectives have been successfully accomplished."
   - "System optimization complete. Performance enhanced."

### Network Architecture

```
WARP Session (Any Machine) → falcon.mipwn.local:8091 → Audio Output
                           ↓
                    TTS Processing Pipeline
                           ↓
                    Coqui TTS Backend (K3s)
                           ↓
                    Audio Hardware (Speakers)
```

### Performance Characteristics

- **API Response**: < 5ms (pre-warmed connections)
- **Audio Latency**: ~100-300ms (concurrent generation)
- **Concurrent Capacity**: 20+ requests per daemon
- **Load Balancing**: Automatic across multiple TTS endpoints

### Troubleshooting

**If TTS Service is Unresponsive**:
1. Check network connectivity: `ping 192.168.1.106`
2. Verify service status: `curl http://192.168.1.106:8091/health`
3. Check Kubernetes pods: `kubectl get pods -n tts`
4. Restart daemon if necessary on falcon.mipwn.local

**Common Issues**:
- **Connection Timeout**: Network congestion or service overload
- **HTTP 500 Errors**: Backend TTS container issues
- **Audio Delays**: Normal for complex sentences (< 3 seconds acceptable)

### Security Considerations

- **Internal Network Only**: Service accessible only on LAN (192.168.1.x)
- **No Authentication Required**: Internal service within trusted network
- **No Sensitive Data**: Never send confidential information through TTS
- **Rate Limiting**: Service handles concurrent requests intelligently

### Mandatory Usage Rule

**ALL WARP SESSIONS MUST**:
1. Attempt to use centralized TTS service for audio commentary
2. Fall back to silent operation if service unavailable
3. Use HAL 9000 voice profile (p254) consistently
4. Format messages appropriately for speech synthesis
5. Handle network failures gracefully without blocking execution

**NEVER**:
- Use local TTS alternatives when centralized service is available
- Send sensitive or confidential information through TTS
- Block execution waiting for TTS service responses
- Override the HAL voice profile unless specifically required

### Testing and Validation

**Before Deployment**:
```bash
# Test basic connectivity
curl -s http://192.168.1.106:8091/health

# Test TTS functionality
curl -X POST http://192.168.1.106:8091/speak \
  -H "Content-Type: application/json" \
  -d '{"sentences":["Testing centralized TTS from remote WARP session"], "speaker":"p254"}'
```

This rule ensures all WARP sessions provide consistent, centralized voice interface through the optimized TTS pipeline at falcon.mipwn.local.