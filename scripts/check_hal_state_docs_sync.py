#!/usr/bin/env python3

from __future__ import annotations

import sys
from pathlib import Path


CHECKS = [
    {
        "path": "README.md",
        "must_contain": [
            "Runtime state persistence is Postgres-backed.",
        ],
        "must_not_contain": [
            "SQLite-backed model is the current implementation",
            "migrate-legacy-state",
        ],
    },
    {
        "path": ".env.template",
        "must_contain": [
            "HALO_STATE_POSTGRES_DSN=",
        ],
        "must_not_contain": [
            "HALO_STATE_BACKEND",
            "HALO_STATE_ENABLE_DUAL_WRITE",
        ],
    },
    {
        "path": "config/halo-state.env.example",
        "must_contain": [
            "HALO_STATE_POSTGRES_DSN=",
        ],
        "must_not_contain": [
            "HALO_STATE_BACKEND",
            "HALO_STATE_ENABLE_DUAL_WRITE",
        ],
    },
    {
        "path": "docs/architecture/aware-state-system.md",
        "must_contain": [
            "HAL aware-mode runtime state is now Postgres-backed.",
            "As of 2026-04-09:",
        ],
        "must_not_contain": [
            "This first pass does not yet replace the runtime logic.",
            "keep the current SQLite-aware implementation running",
            "dual-write sensory events into SQLite and Postgres if needed during cutover",
        ],
    },
    {
        "path": "prompts/halo/prompts-guide.md",
        "must_contain": [
            "aware postgres memory",
            "aware-mode recent and relevant summaries from the Postgres memory store",
        ],
        "must_not_contain": [
            "aware sqlite memory",
            "SQLite memory store",
        ],
    },
    {
        "path": "halo",
        "must_contain": [
            "halo_state_cli",
        ],
        "must_not_contain": [
            "sqlite3",
            "HALO_STATE_BACKEND",
            "HALO_STATE_ENABLE_DUAL_WRITE",
            "AWARE_MEMORY_DB_DEFAULT",
            "COMMENTARY_HISTORY_DB_DEFAULT",
        ],
    },
]


def line_numbers(text: str, token: str) -> str:
    lines = []
    for index, line in enumerate(text.splitlines(), start=1):
        if token in line:
            lines.append(str(index))
    return ", ".join(lines)


def main() -> int:
    repo_root = Path(__file__).resolve().parents[1]
    errors: list[str] = []

    for check in CHECKS:
        path = repo_root / check["path"]
        if not path.exists():
            errors.append(f"missing required file: {check['path']}")
            continue

        text = path.read_text(encoding="utf-8")
        for token in check.get("must_contain", []):
            if token not in text:
                errors.append(f"{check['path']}: missing required text: {token!r}")

        for token in check.get("must_not_contain", []):
            if token in text:
                lines = line_numbers(text, token)
                where = f" on line(s) {lines}" if lines else ""
                errors.append(f"{check['path']}: forbidden text {token!r} found{where}")

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print("HAL state docs/runtime alignment check passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
