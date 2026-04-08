#!/usr/bin/env bash

set -euo pipefail

HALO_WRAPPER_SELF="${BASH_SOURCE[0]:-$0}"
if command -v realpath >/dev/null 2>&1; then
    HALO_WRAPPER_SELF=$(realpath "$HALO_WRAPPER_SELF" 2>/dev/null || printf '%s' "$HALO_WRAPPER_SELF")
fi

APP_ROOT=$(cd -- "$(dirname -- "$HALO_WRAPPER_SELF")/.." && pwd)
COMPOSE_FILE="$APP_ROOT/docker-compose.halo-runtime.yml"
SERVICE_NAME="halo-runtime"
CONTAINER_COMMAND="/opt/halo-app/halo"
HALO_HOST_ROOT="${HALO_ROOT:-/mnt/dkstorage/hal-system-monitor}"
HALO_RUNTIME_UID="${HALO_RUNTIME_UID:-1000}"
HALO_RUNTIME_GID="${HALO_RUNTIME_GID:-1000}"

ensure_host_dir_ownership() {
    local directory="$1"
    local mismatch=""

    sudo -n mkdir -p "$directory"
    mismatch=$(sudo -n find "$directory" \( ! -user "$HALO_RUNTIME_UID" -o ! -group "$HALO_RUNTIME_GID" \) -print -quit 2>/dev/null || true)
    if [ -n "$mismatch" ]; then
        sudo -n chown -R "$HALO_RUNTIME_UID:$HALO_RUNTIME_GID" "$directory"
    fi
}

ensure_host_runtime_dirs() {
    local directory

    if ! sudo -n true >/dev/null 2>&1; then
        return
    fi

    for directory in \
        "$HALO_HOST_ROOT/cache/halo-xtts/podcast" \
        "$HALO_HOST_ROOT/cache/halo-xtts/podcast/bundles" \
        "$HALO_HOST_ROOT/cache/halo-xtts/story" \
        "$HALO_HOST_ROOT/cache/halo-xtts/story/bundles" \
        "$HALO_HOST_ROOT/state/halo/storygen-queue" \
        "$HALO_HOST_ROOT/story" \
        "$HALO_HOST_ROOT/podcast"
    do
        ensure_host_dir_ownership "$directory"
    done
}

run_compose() {
    if docker compose version >/dev/null 2>&1; then
        docker compose -f "$COMPOSE_FILE" "$@"
        return
    fi
    if command -v docker-compose >/dev/null 2>&1; then
        docker-compose -f "$COMPOSE_FILE" "$@"
        return
    fi

    echo "Error: docker compose or docker-compose is required for halo." >&2
    exit 1
}

ensure_host_runtime_dirs

run_compose up -d "$SERVICE_NAME" >/dev/null
if [ -t 0 ] && [ -t 1 ]; then
    run_compose exec "$SERVICE_NAME" "$CONTAINER_COMMAND" "$@"
else
    run_compose exec -T "$SERVICE_NAME" "$CONTAINER_COMMAND" "$@"
fi
exit $?