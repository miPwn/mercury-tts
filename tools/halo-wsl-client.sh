#!/usr/bin/env bash

set -euo pipefail

windows_wrapper="${HALO_WINDOWS_WRAPPER:-/mnt/c/Users/rtmpa/py_scripts/halo.ps1}"
windows_shell="${HALO_WINDOWS_SHELL:-pwsh.exe}"

if [ ! -f "$windows_wrapper" ]; then
  echo "Windows halo wrapper not found at $windows_wrapper" >&2
  exit 1
fi

if ! command -v "$windows_shell" >/dev/null 2>&1; then
  echo "$windows_shell is required for halo WSL client" >&2
  exit 1
fi

wrapper_windows_path="$(wslpath -w "$windows_wrapper")"

exec "$windows_shell" -NoProfile -ExecutionPolicy Bypass -File "$wrapper_windows_path" "$@"