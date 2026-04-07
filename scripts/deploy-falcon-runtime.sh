#!/usr/bin/env bash

set -euo pipefail

usage() {
    cat <<'EOF'
Usage: scripts/deploy-falcon-runtime.sh --runtime halo|hal [--install-path /custom/path] [--install-root /custom/root]
EOF
}

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
RUNTIME_NAME=""
INSTALL_PATH=""
INSTALL_ROOT=""
STAGE_DIR=""

cleanup() {
    if [ -n "$STAGE_DIR" ] && [ -d "$STAGE_DIR" ]; then
        rm -rf "$STAGE_DIR"
    fi
}

trap cleanup EXIT

runtime_payload_paths() {
    case "$1" in
        halo)
            cat <<'EOF'
Dockerfile.halo-runtime
docker-compose.halo-runtime.yml
halo
halo_cache.py
halo_review.py
halo_state
prompts
reference
requirements-state.txt
scripts/halo-container-entrypoint.sh
scripts/halo-container-wrapper.sh
sensory
EOF
            ;;
        hal)
            cat <<'EOF'
hal
EOF
            ;;
        *)
            return 1
            ;;
    esac
}

stage_runtime_payload() {
    local runtime_name="$1"
    local payload_path source_path destination_dir

    STAGE_DIR=$(mktemp -d)
    while IFS= read -r payload_path; do
        [ -n "$payload_path" ] || continue
        source_path="$REPO_ROOT/$payload_path"
        if [ ! -e "$source_path" ]; then
            printf 'Required runtime asset not found: %s\n' "$source_path" >&2
            exit 1
        fi
        destination_dir="$STAGE_DIR/$(dirname "$payload_path")"
        mkdir -p "$destination_dir"
        cp -a "$source_path" "$destination_dir/"
    done < <(runtime_payload_paths "$runtime_name")

    if [ -f "$STAGE_DIR/$runtime_name" ]; then
        chmod +x "$STAGE_DIR/$runtime_name"
    fi
    if [ -f "$STAGE_DIR/scripts/halo-container-entrypoint.sh" ]; then
        chmod +x "$STAGE_DIR/scripts/halo-container-entrypoint.sh"
    fi
    if [ -f "$STAGE_DIR/scripts/halo-container-wrapper.sh" ]; then
        chmod +x "$STAGE_DIR/scripts/halo-container-wrapper.sh"
    fi
}

run_compose() {
    local compose_file="$1"
    shift

    if docker compose version >/dev/null 2>&1; then
        docker compose -f "$compose_file" "$@"
        return
    fi
    if command -v docker-compose >/dev/null 2>&1; then
        docker-compose -f "$compose_file" "$@"
        return
    fi

    printf 'docker compose or docker-compose is required for halo container deployment.\n' >&2
    exit 1
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --runtime)
            RUNTIME_NAME="${2:-}"
            shift 2
            ;;
        --install-path)
            INSTALL_PATH="${2:-}"
            shift 2
            ;;
        --install-root)
            INSTALL_ROOT="${2:-}"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            printf 'Unknown argument: %s\n' "$1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

if [ -z "$RUNTIME_NAME" ]; then
    printf 'Missing required --runtime argument.\n' >&2
    usage >&2
    exit 1
fi

case "$RUNTIME_NAME" in
    halo|hal)
        ;;
    *)
        printf 'Unsupported runtime: %s\n' "$RUNTIME_NAME" >&2
        exit 1
        ;;
esac

if [ -z "$INSTALL_PATH" ]; then
    INSTALL_PATH="/usr/local/bin/${RUNTIME_NAME}"
fi

if [ -z "$INSTALL_ROOT" ]; then
    INSTALL_ROOT="/opt/${RUNTIME_NAME}"
fi

SOURCE_PATH="$REPO_ROOT/$RUNTIME_NAME"
if [ ! -f "$SOURCE_PATH" ]; then
    printf 'Runtime source not found: %s\n' "$SOURCE_PATH" >&2
    exit 1
fi

stage_runtime_payload "$RUNTIME_NAME"

printf '\n==> Installing %s payload to %s\n' "$RUNTIME_NAME" "$INSTALL_ROOT"
sudo -n mkdir -p "$(dirname "$INSTALL_ROOT")" "$(dirname "$INSTALL_PATH")"
sudo -n rm -rf "$INSTALL_ROOT"
sudo -n mkdir -p "$INSTALL_ROOT"
sudo -n cp -a "$STAGE_DIR/." "$INSTALL_ROOT/"

if [ "$RUNTIME_NAME" = "halo" ]; then
    sudo -n ln -sfn "$INSTALL_ROOT/scripts/halo-container-wrapper.sh" "$INSTALL_PATH"

    printf '\n==> Building and starting halo-runtime container\n'
    run_compose "$INSTALL_ROOT/docker-compose.halo-runtime.yml" up -d --build halo-runtime

    printf '\n==> Verifying deployed runtime\n'
    command -v "$RUNTIME_NAME" || true
    sha256sum "$SOURCE_PATH" "$INSTALL_ROOT/halo"
    run_compose "$INSTALL_ROOT/docker-compose.halo-runtime.yml" exec -T halo-runtime sha256sum /opt/halo-app/halo
    run_compose "$INSTALL_ROOT/docker-compose.halo-runtime.yml" ps
    ls -ld "$INSTALL_ROOT"
    ls -l "$INSTALL_PATH"
    exit 0
fi

sudo -n ln -sfn "$INSTALL_ROOT/$RUNTIME_NAME" "$INSTALL_PATH"

printf '\n==> Verifying deployed runtime\n'
command -v "$RUNTIME_NAME" || true
sha256sum "$SOURCE_PATH" "$INSTALL_ROOT/$RUNTIME_NAME"
ls -ld "$INSTALL_ROOT"
ls -l "$INSTALL_PATH"
