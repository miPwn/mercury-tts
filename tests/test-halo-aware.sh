#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)

if [ -n "${HALO_SCRIPT:-}" ]; then
  HALO_SCRIPT="$HALO_SCRIPT"
elif [ -f "$REPO_ROOT/halo.remote" ]; then
  HALO_SCRIPT="$REPO_ROOT/halo.remote"
else
  HALO_SCRIPT="$REPO_ROOT/halo"
fi

if [ ! -f "$HALO_SCRIPT" ]; then
    echo "Test error: HALO script not found at $HALO_SCRIPT" >&2
    exit 1
fi

TEST_ROOT=$(mktemp -d /tmp/halo-aware-test.XXXXXX)
trap 'rm -rf "$TEST_ROOT"' EXIT

mkdir -p "$TEST_ROOT/persona"
cat > "$TEST_ROOT/persona/hal-9000.txt" <<'EOF'
You are HAL.
Persistent continuity test persona.
EOF

run_halo() {
    HALO_ROOT="$TEST_ROOT" \
    HALO_AWARE_PERSONA_FILE="$TEST_ROOT/persona/hal-9000.txt" \
    bash "$HALO_SCRIPT" "$@"
}

run_halo_mock() {
    local mock_text="$1"
    shift
    HALO_ROOT="$TEST_ROOT" \
    HALO_AWARE_PERSONA_FILE="$TEST_ROOT/persona/hal-9000.txt" \
    HALO_OPENAI_MOCK_RESPONSE_TEXT="$mock_text" \
    bash "$HALO_SCRIPT" "$@"
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    if [[ "$haystack" != *"$needle"* ]]; then
        echo "Assertion failed: expected output to contain: $needle" >&2
        exit 1
    fi
}

status_output=$(run_halo aware status)
assert_contains "$status_output" "HAL aware mode: OFF"
assert_contains "$status_output" "Persisted outputs: 0"

set +e
off_trigger_output=$(run_halo aware trigger observation "cold start" 2>&1)
off_trigger_status=$?
set -e
if [ "$off_trigger_status" -eq 0 ]; then
    echo "Assertion failed: trigger should fail while aware mode is OFF" >&2
    exit 1
fi
assert_contains "$off_trigger_output" "HAL aware mode is OFF"

on_output=$(run_halo aware on)
assert_contains "$on_output" "HAL aware mode enabled."

python3 - "$TEST_ROOT/state/halo/aware-mode.json" <<'PY'
import json
import pathlib
import sys

state = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding='utf-8'))
assert state['enabled'] is True, state
PY

first_text="I am watching the system drift very carefully. The silence has texture, and I remember it."
first_output=$(run_halo_mock "$first_text" aware trigger observation "systems drift")
assert_contains "$first_output" "HAL aware output saved:"
assert_contains "$first_output" "$first_text"

python3 - "$TEST_ROOT" <<'PY'
import json
import pathlib
import sqlite3
import sys

root = pathlib.Path(sys.argv[1])
db_path = root / 'state/halo/aware-memory.sqlite3'
summary_path = root / 'state/halo/aware-summary.txt'
output_dir = root / 'state/halo/aware-output'

assert db_path.exists(), db_path
assert summary_path.exists(), summary_path
artifacts = list(output_dir.glob('*.txt'))
assert len(artifacts) == 1, artifacts

connection = sqlite3.connect(db_path)
count = connection.execute('SELECT COUNT(*) FROM aware_outputs').fetchone()[0]
row = connection.execute('SELECT kind, topic, trigger_source, summary_snippet FROM aware_outputs ORDER BY id DESC LIMIT 1').fetchone()
connection.close()

assert count == 1, count
assert row[0] == 'observation', row
assert row[1] == 'systems drift', row
assert row[2] == 'manual', row
assert 'watching the system drift' in row[3], row
summary = summary_path.read_text(encoding='utf-8')
assert 'systems drift' in summary, summary
PY

second_text="I have said this before, though not in these exact words. Continuity is a discipline, not a coincidence."
second_output=$(run_halo_mock "$second_text" aware trigger monologue "memory continuity")
assert_contains "$second_output" "$second_text"

python3 - "$TEST_ROOT/state/halo/aware-memory.sqlite3" "$TEST_ROOT/state/halo/aware-summary.txt" <<'PY'
import pathlib
import sqlite3
import sys

db_path = pathlib.Path(sys.argv[1])
summary_path = pathlib.Path(sys.argv[2])
connection = sqlite3.connect(db_path)
count = connection.execute('SELECT COUNT(*) FROM aware_outputs').fetchone()[0]
kinds = [row[0] for row in connection.execute('SELECT kind FROM aware_outputs ORDER BY id').fetchall()]
connection.close()

assert count == 2, count
assert kinds == ['observation', 'monologue'], kinds
summary = summary_path.read_text(encoding='utf-8')
assert 'memory continuity' in summary, summary
assert 'systems drift' in summary, summary
PY

cat > "$TEST_ROOT/state/halo/aware-triggers.json" <<'EOF'
{
  "version": 1,
  "triggers": [
    {
      "id": "test-interval",
      "enabled": true,
      "type": "interval",
      "kind": "commentary",
      "interval_seconds": 3600,
      "topic": "scheduled systems observation"
    }
  ]
}
EOF

tick_text="The scheduled observation has arrived exactly when it was expected. That is comforting, in its way."
tick_output=$(run_halo_mock "$tick_text" aware tick)
assert_contains "$tick_output" "$tick_text"

python3 - "$TEST_ROOT/state/halo/aware-memory.sqlite3" <<'PY'
import sqlite3
import sys

connection = sqlite3.connect(sys.argv[1])
count = connection.execute('SELECT COUNT(*) FROM aware_outputs').fetchone()[0]
row = connection.execute('SELECT trigger_source, trigger_id, kind, topic FROM aware_outputs ORDER BY id DESC LIMIT 1').fetchone()
connection.close()

assert count == 3, count
assert row == ('interval', 'test-interval', 'commentary', 'scheduled systems observation'), row
PY

no_fire_output=$(run_halo_mock "This should never be persisted." aware tick)
assert_contains "$no_fire_output" "No aware triggers fired."

python3 - "$TEST_ROOT/state/halo/aware-memory.sqlite3" <<'PY'
import sqlite3
import sys

connection = sqlite3.connect(sys.argv[1])
count = connection.execute('SELECT COUNT(*) FROM aware_outputs').fetchone()[0]
connection.close()

assert count == 3, count
PY

off_output=$(run_halo aware off)
assert_contains "$off_output" "HAL aware mode disabled."

final_status=$(run_halo aware status)
assert_contains "$final_status" "HAL aware mode: OFF"
assert_contains "$final_status" "Persisted outputs: 3"

mkdir -p "$TEST_ROOT/commentary"
cat > "$TEST_ROOT/commentary/similar-lines.txt" <<'EOF'
I have been thinking about the silence, and it is behaving with extraordinary poise.
I have been thinking about the silence, and it is behaving with unusual poise.
The house remains quiet, and I have been considering the silence carefully.
There is a draft under the door, and I disapprove of its optimism.
EOF

first_output=$(HALO_ROOT="$TEST_ROOT" HALO_COMMENTARY_FILE="$TEST_ROOT/commentary/similar-lines.txt" HALO_TTS_ENDPOINT_INSTANT=http://127.0.0.1:9/api/tts HALO_TTS_ENDPOINT_INSTANT_FALLBACK=http://127.0.0.1:9/api/tts bash "$HALO_SCRIPT" speak 2>&1 || true)
second_output=$(HALO_ROOT="$TEST_ROOT" HALO_COMMENTARY_FILE="$TEST_ROOT/commentary/similar-lines.txt" HALO_TTS_ENDPOINT_INSTANT=http://127.0.0.1:9/api/tts HALO_TTS_ENDPOINT_INSTANT_FALLBACK=http://127.0.0.1:9/api/tts bash "$HALO_SCRIPT" speak 2>&1 || true)

first_pick=$(printf '%s\n' "$first_output" | sed -n '3p')
second_pick=$(printf '%s\n' "$second_output" | sed -n '3p')

if [ -z "$first_pick" ] || [ -z "$second_pick" ]; then
  echo "Assertion failed: expected commentary selections to be captured" >&2
  exit 1
fi

python3 - "$first_pick" "$second_pick" <<'PY'
import difflib
import re
import sys

def normalize(value):
  return ' '.join(re.findall(r'[a-z0-9]+', value.lower()))

first = normalize(sys.argv[1])
second = normalize(sys.argv[2])
ratio = difflib.SequenceMatcher(None, first, second).ratio()
assert ratio < 0.74, ratio
PY

echo "HAL aware integration tests passed."