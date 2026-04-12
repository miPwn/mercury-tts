# Mercury TTS

`mercury-tts` is the speech-rendering service for HAL.

Its responsibility is narrow:

- accept text or sentence batches over HTTP
- render speech using the configured TTS backends
- return or queue playback work
- expose health and service diagnostics for the speech path

This repo no longer owns:

- runtime orchestration
- chat workflow
- memory/state management
- aware mode
- sensory processing
- review or story generation
- canon or prompt governance

Those concerns belong in other services:

- `hal-orchestrator`
- `hal-memory-fabric`
- `halo-chat`
- `hal-voice-recognition`

## Active Surface

Primary local service endpoints:

- `GET /health`
- `POST /speak`

The current operator helper script is:

- [`hal`](F:/DEVELOPMENT/FALCON_LOCAL/halo-system/mercury-tts/hal)

That script is a thin TTS client only. It is not a runtime shell.

## Development

Run Go validation:

```bash
go test ./...
```

Important files:

- [`instant_tts_daemon.go`](F:/DEVELOPMENT/FALCON_LOCAL/halo-system/mercury-tts/instant_tts_daemon.go)
- [`speak_client.go`](F:/DEVELOPMENT/FALCON_LOCAL/halo-system/mercury-tts/speak_client.go)
- [`Dockerfile.hal-tts`](F:/DEVELOPMENT/FALCON_LOCAL/halo-system/mercury-tts/Dockerfile.hal-tts)
- [`hal-tts.service`](F:/DEVELOPMENT/FALCON_LOCAL/halo-system/mercury-tts/hal-tts.service)

## Notes

Older runtime/state assets were removed from this repo during the orchestrator split. If you need HAL workflow behavior, use `hal-orchestrator`, not `mercury-tts`.
