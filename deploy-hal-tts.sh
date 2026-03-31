#!/bin/bash

# Deploy HAL TTS service to k3s
# This script builds the Docker image and deploys it to the cluster

set -e

echo "🚀 Deploying HAL TTS service to k3s cluster..."

# Step 1: Build the Docker image
echo "📦 Building HAL TTS Docker image..."
echo "🔨 Rebuilding HAL TTS daemon binary..."
go build -o streaming_safe_daemon ./streaming_safe_daemon.go
docker build -f Dockerfile.hal-tts -t hal-tts-streaming:latest .

# Step 2: Load image into k3s (since k3s uses containerd)
echo "📥 Loading image into k3s..."
docker save hal-tts-streaming:latest | sudo k3s ctr images import -

# Step 3: Create the tts namespace if it doesn't exist
echo "🏗️ Ensuring TTS namespace exists..."
kubectl create namespace tts --dry-run=client -o yaml | kubectl apply -f -

# Step 4: Stop the local systemd service
echo "🛑 Stopping local HAL TTS service..."
sudo systemctl stop hal-tts.service
sudo systemctl disable hal-tts.service

# Step 5: Deploy to k3s
echo "🚀 Deploying to k3s cluster..."
kubectl apply -f k8s/hal-tts-deployment.yaml
echo "🔄 Restarting deployment to pick up refreshed local image..."
kubectl rollout restart deployment/hal-tts -n tts

# Step 6: Wait for deployment to be ready
echo "⏳ Waiting for deployment rollout to complete..."
kubectl rollout status deployment/hal-tts -n tts --timeout=300s

# Step 7: Show status
echo "📊 Deployment status:"
kubectl get pods -n tts -l app=hal-tts
kubectl get svc -n tts hal-tts

echo "✅ HAL TTS service deployed to k3s successfully!"
echo "🎙️ HAL-006 voice should now be available through the k3s cluster"
echo "🔗 Service will be available at: http://192.168.1.106:8091"