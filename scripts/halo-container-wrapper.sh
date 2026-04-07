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

run_compose up -d "$SERVICE_NAME" >/dev/null
if [ -t 0 ] && [ -t 1 ]; then
    run_compose exec "$SERVICE_NAME" "$CONTAINER_COMMAND" "$@"
else
    run_compose exec -T "$SERVICE_NAME" "$CONTAINER_COMMAND" "$@"
fi
exit $?