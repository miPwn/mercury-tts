#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)

HALO_SCRIPT="${HALO_SCRIPT:-$REPO_ROOT/halo}"
if [ ! -f "$HALO_SCRIPT" ]; then
    echo "Test error: HALO script not found at $HALO_SCRIPT" >&2
    exit 1
fi

TEST_ROOT=$(mktemp -d /tmp/halo-review-test.XXXXXX)
trap 'rm -rf "$TEST_ROOT"' EXIT

mkdir -p "$TEST_ROOT/persona"
cat > "$TEST_ROOT/persona/hal-9000.txt" <<'EOF'
You are HAL.
Persistent review test persona.
EOF

cat > "$TEST_ROOT/reject-me.txt" <<'EOF'
systemd[1]: starting unit
cpu=4% mem=68%
ssh falcon.mipwn.local failed
kubectl get svc -A | Select-String coqui-xtts
EOF

OUTPUT_LOG="$TEST_ROOT/output.log"
HALO_ROOT="$TEST_ROOT" HALO_AWARE_PERSONA_FILE="$TEST_ROOT/persona/hal-9000.txt" HALO_REVIEW_PRECHECK_MOCK_DECISION=fail bash "$HALO_SCRIPT" review "$TEST_ROOT/reject-me.txt" >"$OUTPUT_LOG" 2>&1 || true
exact=$(tail -n 1 "$OUTPUT_LOG" | tr -d '\r')

if [ "$exact" != "source material rejection" ]; then
    echo "Assertion failed: expected exact rejection message, got: $exact" >&2
    exit 1
fi

echo "HAL review rejection test passed."