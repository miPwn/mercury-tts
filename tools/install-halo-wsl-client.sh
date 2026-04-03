#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source_client="$script_dir/halo-wsl-client.sh"
target_dir="$HOME/bin"
target_client="$target_dir/halo"
profile_file="$HOME/.profile"
path_line='export PATH="$HOME/bin:$PATH"'

mkdir -p "$target_dir"
install -m 755 "$source_client" "$target_client"

if ! printf '%s' ":$PATH:" | grep -Fq ":$HOME/bin:"; then
  touch "$profile_file"
  if ! grep -Fqx "$path_line" "$profile_file"; then
    printf '\n%s\n' "$path_line" >> "$profile_file"
  fi
fi

printf 'Installed halo WSL client to %s\n' "$target_client"
printf 'Windows wrapper: %s\n' "${HALO_WINDOWS_WRAPPER:-/mnt/c/Users/rtmpa/py_scripts/halo.ps1}"
printf 'Windows shell: %s\n' "${HALO_WINDOWS_SHELL:-pwsh.exe}"
printf 'Run with: halo /?\n'