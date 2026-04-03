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

TEST_ROOT=$(mktemp -d /tmp/halo-sensory-test.XXXXXX)
trap 'rm -rf "$TEST_ROOT"' EXIT

mkdir -p "$TEST_ROOT/persona"
cat > "$TEST_ROOT/persona/hal-9000.txt" <<'EOF'
You are HAL.
You are observational, analytical, and grounded in the system around you.
EOF

run_halo() {
    HALO_ROOT="$TEST_ROOT" \
    HALO_AWARE_PERSONA_FILE="$TEST_ROOT/persona/hal-9000.txt" \
    PYTHONPATH="$REPO_ROOT${PYTHONPATH:+:$PYTHONPATH}" \
    bash "$HALO_SCRIPT" "$@"
}

run_halo_mock() {
    local mock_text="$1"
    shift
    HALO_ROOT="$TEST_ROOT" \
    HALO_AWARE_PERSONA_FILE="$TEST_ROOT/persona/hal-9000.txt" \
    HALO_OPENAI_MOCK_RESPONSE_TEXT="$mock_text" \
    PYTHONPATH="$REPO_ROOT${PYTHONPATH:+:$PYTHONPATH}" \
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

status_output=$(run_halo sensory status)
assert_contains "$status_output" '"available_sensors"'

scan_output=$(run_halo sensory scan host)
assert_contains "$scan_output" '"sensor_name": "host"'

post_scan_status=$(run_halo sensory status)
assert_contains "$post_scan_status" '"observation_count": 1'

run_halo aware on >/dev/null

commentary_text="I can feel the host body in measurable terms. Its rhythms are not mysterious, merely persistent."
commentary_output=$(run_halo_mock "$commentary_text" sensory commentary host)
assert_contains "$commentary_output" '"should_generate": true'
assert_contains "$commentary_output" "$commentary_text"

python3 - "$TEST_ROOT/state/halo/aware-memory.sqlite3" "$TEST_ROOT/state/halo/sensory/knowledge.sqlite3" <<'PY'
import sqlite3
import sys

aware_db = sqlite3.connect(sys.argv[1])
aware_count = aware_db.execute('SELECT COUNT(*) FROM aware_outputs').fetchone()[0]
aware_db.close()

sensory_db = sqlite3.connect(sys.argv[2])
history_count = sensory_db.execute('SELECT COUNT(*) FROM commentary_history').fetchone()[0]
sensory_db.close()

assert aware_count == 1, aware_count
assert history_count == 1, history_count
PY

second_output=$(run_halo_mock "This should be suppressed." sensory commentary host)
assert_contains "$second_output" '"should_generate": false'
assert_contains "$second_output" '"reason": "cooldown"'

python3 - "$TEST_ROOT/state/halo/aware-memory.sqlite3" <<'PY'
import sqlite3
import sys

aware_db = sqlite3.connect(sys.argv[1])
aware_count = aware_db.execute('SELECT COUNT(*) FROM aware_outputs').fetchone()[0]
aware_db.close()
assert aware_count == 1, aware_count
PY

PYTHONPATH="$REPO_ROOT${PYTHONPATH:+:$PYTHONPATH}" python3 -m unittest discover -s "$REPO_ROOT/tests" -p 'test_halo_sensory.py'

echo "HAL sensory tests passed."