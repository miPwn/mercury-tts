#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" && pwd)
REPO_ROOT=$SCRIPT_DIR
IMAGE_NAME="${HAL_TTS_IMAGE_NAME:-hal-tts-streaming}"
IMAGE_TAG="${HAL_TTS_IMAGE_TAG:-latest}"
FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"
K3S_NAMESPACE="${HAL_TTS_NAMESPACE:-tts}"
K3S_DEPLOYMENT="${HAL_TTS_DEPLOYMENT:-hal-tts}"
SERVICE_NAME="${HAL_TTS_SERVICE_NAME:-hal-tts}"

log_step() {
	printf '\n==> %s\n' "$1"
}

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		printf 'Required command not found: %s\n' "$1" >&2
		exit 1
	fi
}

require_command go
require_command docker
require_command sudo

cd "$REPO_ROOT"

log_step "Building HAL TTS daemon"
go build -o streaming_safe_daemon ./streaming_safe_daemon.go

log_step "Building Docker image ${FULL_IMAGE}"
docker build -f Dockerfile.hal-tts -t "$FULL_IMAGE" .

log_step "Importing image into k3s"
docker save "$FULL_IMAGE" | sudo -n k3s ctr images import -

log_step "Ensuring namespace ${K3S_NAMESPACE} exists"
sudo -n k3s kubectl create namespace "$K3S_NAMESPACE" --dry-run=client -o yaml | sudo -n k3s kubectl apply -f -

if systemctl list-unit-files hal-tts.service >/dev/null 2>&1; then
	log_step "Stopping legacy host hal-tts.service"
	sudo -n systemctl stop hal-tts.service || true
	sudo -n systemctl disable hal-tts.service || true
fi

log_step "Applying HAL TTS manifests"
sudo -n k3s kubectl apply -f k8s/hal-tts-deployment.yaml
sudo -n k3s kubectl set image "deployment/${K3S_DEPLOYMENT}" hal-tts="$FULL_IMAGE" -n "$K3S_NAMESPACE"

log_step "Restarting deployment"
sudo -n k3s kubectl rollout restart "deployment/${K3S_DEPLOYMENT}" -n "$K3S_NAMESPACE"

log_step "Waiting for rollout"
sudo -n k3s kubectl rollout status "deployment/${K3S_DEPLOYMENT}" -n "$K3S_NAMESPACE" --timeout=300s

log_step "Deployment status"
sudo -n k3s kubectl get pods -n "$K3S_NAMESPACE" -l app="$K3S_DEPLOYMENT"
sudo -n k3s kubectl get svc -n "$K3S_NAMESPACE" "$SERVICE_NAME"

printf '\nHAL TTS service deployed with image %s\n' "$FULL_IMAGE"