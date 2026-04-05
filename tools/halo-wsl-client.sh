#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
windows_wrapper_default="$(wslpath -w "$script_dir/halo.ps1")"
windows_wrapper="${HALO_WINDOWS_WRAPPER:-$windows_wrapper_default}"
windows_shell="${HALO_WINDOWS_SHELL:-pwsh.exe}"

show_help() {
  cat <<EOF
HALO command client

Usage:
  halo 'your message here'
  halo [command] [options]

General:
  halo /?
  halo -l [--json]
  halo vq
  halo speak

Playback and review:
  halo read <story-name|filename.txt|/full/path/to/file.txt>
  halo review <name|filename.txt|/full/path/to/file.txt>

Generation:
  halo storygen [-mw max_words | -pc minutes] [topic]
  halo storygen -rc
    aliases: halo story-gen, halo sg
    notes: -mw and -pc are mutually exclusive; requests enqueue immediately

Aware mode:
  halo aware on|off|status
  halo aware trigger [commentary|observation|monologue|story] [topic]
  halo aware tick

Sensory:
  halo sensory status
  halo sensory scan [host|network|photoprism|blink|all]
  halo sensory commentary [host|network|photoprism|blink|all]

Render-only variants:
  halo --render-only 'sentence'
  halo --render-only read <story-name|filename.txt|/full/path/to/file.txt>
  halo --render-only review <name|filename.txt|/full/path/to/file.txt>
  halo --render-only storygen [-mw max_words | -pc minutes] [topic]

WSL wrapper details:
  Windows wrapper: ${windows_wrapper}
  Windows shell:   ${windows_shell}
  This client forwards all non-help commands to the Windows halo wrapper.
EOF
}

case "${1:-}" in
  /?|--help|-h|help)
    show_help
    exit 0
    ;;
esac

if [ ! -f "$windows_wrapper" ]; then
  echo "Windows halo wrapper not found at $windows_wrapper" >&2
  echo "Run 'halo /?' for wrapper usage." >&2
  exit 1
fi

if ! command -v "$windows_shell" >/dev/null 2>&1; then
  echo "$windows_shell is required for halo WSL client" >&2
  echo "Run 'halo /?' for wrapper usage." >&2
  exit 1
fi

wrapper_windows_path="$(wslpath -w "$windows_wrapper")"

exec "$windows_shell" -NoProfile -ExecutionPolicy Bypass -File "$wrapper_windows_path" "$@"