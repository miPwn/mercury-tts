#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" && pwd)
WORKSPACE_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
RELEASE_ID="${HAL_RELEASE_ID:-$(date +%Y%m%d-%H%M%S)}"
MESH_CONFIG="${HAL_MESH_CONFIG:-$WORKSPACE_ROOT/mesh/mesh.halo.yaml}"
MESH_EXE="${HAL_MESH_EXE:-$WORKSPACE_ROOT/mesh/target/release/mesh.exe}"
export PYTHONPATH="$WORKSPACE_ROOT/hal-platform-ops/src"

python -m hal_platform_ops release \
  --environment falcon \
  --release-id "$RELEASE_ID" \
  --workspace-root "$WORKSPACE_ROOT" \
  --mesh-exe "$MESH_EXE" \
  --mesh-config "$MESH_CONFIG" \
  --service hal-tts
