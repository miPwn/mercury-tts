#!/usr/bin/env bash

set -euo pipefail

HALO_ROOT_DIR="${HALO_ROOT:-/mnt/dkstorage/hal-system-monitor}"

mkdir -p \
    "$HALO_ROOT_DIR/cache/halo-xtts" \
    "$HALO_ROOT_DIR/story" \
    "$HALO_ROOT_DIR/podcast" \
    "$HALO_ROOT_DIR/review" \
    "$HALO_ROOT_DIR/commentary" \
    "$HALO_ROOT_DIR/persona" \
    "$HALO_ROOT_DIR/state/halo/storygen-queue/pending" \
    "$HALO_ROOT_DIR/state/halo/storygen-queue/processing" \
    "$HALO_ROOT_DIR/state/halo/storygen-queue/done" \
    "$HALO_ROOT_DIR/state/halo/storygen-queue/failed" \
    "$HALO_ROOT_DIR/state/halo/sensory"

if [ "$#" -gt 0 ]; then
    exec "$@"
fi

exec sleep infinity