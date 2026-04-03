from __future__ import annotations

from typing import Any

try:
    import psycopg
    from psycopg.rows import dict_row
except ImportError:  # pragma: no cover - dependency optional at import time
    psycopg = None
    dict_row = None


def require_psycopg() -> Any:
    if psycopg is None:
        raise RuntimeError(
            "psycopg is required for HAL Postgres state operations. "
            "Install dependencies from requirements-state.txt."
        )
    return psycopg


def connect(dsn: str):
    module = require_psycopg()
    return module.connect(dsn, row_factory=dict_row)
