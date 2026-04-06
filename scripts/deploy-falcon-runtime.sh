#!/usr/bin/env bash

set -euo pipefail

usage() {
    cat <<'EOF'
Usage: scripts/deploy-falcon-runtime.sh --runtime halo|hal [--install-path /custom/path]
EOF
}

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
RUNTIME_NAME=""
INSTALL_PATH=""

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

SOURCE_PATH="$REPO_ROOT/$RUNTIME_NAME"
if [ ! -f "$SOURCE_PATH" ]; then
    printf 'Runtime source not found: %s\n' "$SOURCE_PATH" >&2
    exit 1
fi

chmod +x "$SOURCE_PATH"

printf '\n==> Installing %s to %s\n' "$RUNTIME_NAME" "$INSTALL_PATH"
sudo -n install -m 0755 "$SOURCE_PATH" "$INSTALL_PATH"

printf '\n==> Verifying deployed runtime\n'
command -v "$RUNTIME_NAME" || true
sha256sum "$SOURCE_PATH" "$INSTALL_PATH"
ls -l "$INSTALL_PATH"
