# System Specifications and Project Documentation Rule

## User Profile
- **Name**: Rich (hacker alias: mipwn)
- **Domain**: mipwn.com (hosted on Cloudflare)
- **Operating System**: Ubuntu Linux
- **Shell**: bash (version 5.2.21)
- **Home Directory**: /home/mipwn

## Hardware Infrastructure
- **Primary Network**: Starlink router in modem-only mode → Draytek 2927ax router
- **Network Equipment**: 
  - 2x Netgear switches (including GS324)
  - 2x range extenders
  - 1x TP-Link router (extension mode)
- **Audio**: Multiple audio devices including Komplete Audio 6 (card 5), USB Audio Device (card 1)

## Containerization & Orchestration
- **Container Runtime**: Docker + k3s Kubernetes cluster
- **Cluster Node**: falcon (192.168.1.106)
- **Registry**: Local containerd for k3s images

## Services & Applications
### Core Infrastructure
- **Portainer**: Port 9000 (container management)
- **Pi-hole**: Port 8070 (DNS filtering)
- **k3s**: Kubernetes cluster for service orchestration

### TTS Pipeline Services
- **HAL TTS Service**: 
  - **Location**: k3s cluster (tts namespace)
  - **Port**: 8091
  - **Voice Model**: HAL-006 (p254 speaker profile)
  - **Audio**: PulseAudio integration via socket mount
  - **Image**: hal-tts-streaming:latest (local build)
- **Standard TTS**: Port 5002 (VCTK model, k3s deployment)

### Domain Subdomains (mipwn.com)
- jedah, photo, pydio, chat, dashboard, food, notes, calendar, wiki
- lidarr, monitor, torrent, nzb, logs, ytdl, data, dns
- portainer, navidrome

## Development Projects & Repositories

### Mercury TTS Pipeline (/home/mipwn/dev/tts-pipeline)
**Status**: PRODUCTION READY v1.2.0 - Public Release (AGPL-3.0)
**GitHub**: https://github.com/miPwn/mercury-tts
**Technologies**: Docker, Kubernetes, Go, Python, ALSA/PulseAudio
**Components**:
- `streaming_safe_daemon`: Core HAL TTS binary
- `hal` script: Voice command interface with comma-separated sentence support
- `HALO` script: Audio generation without playback
- Voice cloning models in `voice-cloning/` directory
- Docker containers: `hal-tts-streaming:latest`, `hal-tts-server:latest`
- k3s manifests in `k8s/hal-tts-deployment.yaml`
- GitHub Actions CI/CD pipeline

### HAL System Monitor (/home/mipwn/dev/hal-system-monitor)
**Status**: PRODUCTION READY v1.2.0 - Public Release (AGPL-3.0)
**GitHub**: https://github.com/miPwn/hal-sysmon
**Technologies**: Go, systemd, GPT-4 integration, TTS pipeline
**Components**:
- `hal-sysmon`: Core monitoring daemon
- `halo-mon`: CLI management interface with HAL 9000 dashboard
- Natural language alert creation via GPT-4
- Dual voice system (HALO/HAL)
- Real-time system monitoring (CPU, Memory, Disk, Network, Docker)
- GitHub Actions CI/CD pipeline

**Recent Work (Completed)**:
- ✅ Successfully migrated HAL TTS from systemd to k3s with PulseAudio socket mounting
- ✅ Containerized streaming_safe_daemon with complete audio device access
- ✅ Deployed HAL-006 voice model in k3s cluster (hal-tts-streaming:latest)
- ✅ Resolved container audio access via PulseAudio unix socket integration
- ✅ Updated HAL script to work with containerized service (port 8091, p254 speaker)
- ✅ Updated HALO script to use HAL-006 voice through containerized service
- ✅ Both HAL and HALO commands now fully operational with k3s deployment

**PUBLIC RELEASE PREPARATION (Oct 11, 2025)**:
- ✅ **SECURITY**: Removed ALL personal IP addresses, API keys, and user references
- ✅ **LICENSE**: Both repos converted to AGPL-3.0 for open source compliance  
- ✅ **CI/CD**: GitHub Actions pipelines with testing, security scanning, release automation
- ✅ **BRANCHES**: Migrated master → main for modern standards
- ✅ **TAGS**: v1.2.0 releases created for both Mercury TTS and HAL System Monitor
- ✅ **REPOS**: Both repositories fully public-ready with professional documentation

## Development Environment Preferences
- **Virtual Environments**: Prefers venv to avoid breaking local development
- **Documentation**: Comprehensive documentation required for all repositories
- **Testing Strategy**: Requires detailed testing documentation
- **Alert Strategy**: Minimal alerts (only critical issues like failing pods/CPU overload)

## Voice & TTS Configuration
- **TTS Clients**: 
  - `hal` command (alias to /home/mipwn/dev/tts-pipeline/hal) - Fast streaming TTS
  - `halo` command (alias to /home/mipwn/dev/tts-pipeline/halo) - HAL-006 authentic voice cloning
- **Voice Profiles**: 
  - HAL: p254 speaker configuration (streaming)
  - HALO: HAL-006 voice model (authentic cloning)
- **Audio Backend**: PulseAudio server via unix socket
- **Speech Rules**: 
  - All responses must be spoken using `hal "sentence 1" "sentence 2" "sentence 3"`
  - Separate quoted arguments enable sentence chunking for concurrent TTS generation
  - This ensures faster processing and smoother sequential playback
  - HAL 9000 persona with calm, measured delivery

## Network & Security
- **MX Records**: Managed via Cloudflare
- **Internal Services**: Accessible via 192.168.1.106 (falcon node)
- **Container Registry**: Local containerd images for k3s
- **Audio Group**: GID 29 for container audio device access

## Current Technical Challenges Resolved
1. ✅ Container audio access via PulseAudio socket mounting
2. ✅ HAL-006 voice model deployment in k3s
3. ✅ TTS service migration from systemd to Kubernetes
4. ✅ Permission issues with voice-cloning directory access
5. ✅ HAL command integration with containerized service
6. ✅ HALO command integration with HAL-006 voice cloning
7. ✅ Complete TTS pipeline containerization with audio output
8. ✅ **CRITICAL**: Exposed OpenAI API key removal from HAL System Monitor
9. ✅ Complete network topology sanitization for public release
10. ✅ Professional CI/CD pipeline implementation for both repositories

## Current System Status (Production Ready)
- **Mercury TTS Pipeline v1.2.0**: Public repository, AGPL-3.0, CI/CD active
- **HAL System Monitor v1.2.0**: Public repository, AGPL-3.0, CI/CD active  
- **Voice System**: Fully operational (HAL + HALO commands)
- **Infrastructure**: k3s cluster with HAL-006 voice model
- **Security**: All personal information sanitized for public use
- **Audio**: EasyEffects configuration pending for voice enhancement

---
*Last Updated: 2025-10-11 - MAJOR: Public release v1.2.0 security sanitization complete*
*Session Status: Both repositories production-ready, secure, and publicly available*
