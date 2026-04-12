from __future__ import annotations

import json
from pathlib import Path
from .config import StateConfig
from .postgres import connect
from .utils import build_memory_payload, build_summary_text, ensure_parent, mask_dsn, now_iso, summarize_text


class PostgresStateService:
    def __init__(self, config: StateConfig) -> None:
        self.config = config
        if not self.config.postgres_dsn:
            raise RuntimeError("HALO_STATE_POSTGRES_DSN is not configured.")
        self.schema = self.config.postgres_schema

    def _profile_id(self, connection) -> str:
        with connection.cursor() as cursor:
            cursor.execute(
                f"SELECT id FROM {self.schema}.identity_profiles WHERE profile_key = %s",
                (self.config.profile_key,),
            )
            row = cursor.fetchone()
            if row:
                return row["id"]
            cursor.execute(
                f"""
                INSERT INTO {self.schema}.identity_profiles (profile_key, display_name, description, persona_text)
                VALUES (%s, %s, %s, %s)
                RETURNING id
                """,
                (
                    self.config.profile_key,
                    self.config.profile_key.upper(),
                    "Auto-created HAL identity profile.",
                    "",
                ),
            )
            created = cursor.fetchone()
            connection.commit()
            return created["id"]

    def status(
        self,
        state_file: Path,
        summary_file: Path,
        trigger_file: Path,
        output_dir: Path,
        legacy_memory_db: Path,
    ) -> str:
        state = {}
        if state_file.exists():
            try:
                state = json.loads(state_file.read_text(encoding="utf-8"))
            except Exception:
                state = {}

        with connect(self.config.postgres_dsn) as connection:
            profile_id = self._profile_id(connection)
            with connection.cursor() as cursor:
                cursor.execute(
                    f"SELECT COUNT(*) AS total FROM {self.schema}.generated_artifacts WHERE profile_id = %s",
                    (profile_id,),
                )
                total_outputs = int(cursor.fetchone()["total"])

        lines = [
            f"HAL aware mode: {'ON' if state.get('enabled') else 'OFF'}",
            f"Persona file: {state.get('persona_file') or 'unknown'}",
            f"Memory store: {mask_dsn(self.config.postgres_dsn)} [{self.config.postgres_schema}]",
            f"Legacy memory db: {legacy_memory_db}",
            f"Summary file: {summary_file}",
            f"Trigger config: {trigger_file}",
            f"Output archive: {output_dir}",
            f"Persisted outputs: {total_outputs}",
        ]
        if summary_file.exists():
            lines.append(f"Summary status: present ({summary_file.stat().st_size} bytes)")
        else:
            lines.append("Summary status: not built yet")
        return "\n".join(lines)

    def _memory_rows(self, limit: int) -> list[dict[str, str]]:
        with connect(self.config.postgres_dsn) as connection:
            profile_id = self._profile_id(connection)
            with connection.cursor() as cursor:
                cursor.execute(
                    f"""
                    SELECT
                        e.observed_at::text AS created_at,
                        e.event_type AS kind,
                        e.event_source AS trigger_source,
                        e.topic,
                        e.body AS text,
                        e.summary AS summary_snippet,
                        COALESCE(a.file_path, '') AS artifact_path
                    FROM {self.schema}.memory_events e
                    LEFT JOIN {self.schema}.generated_artifacts a
                      ON a.id = e.source_artifact_id
                    WHERE e.profile_id = %s
                    ORDER BY e.observed_at DESC
                    LIMIT %s
                    """,
                    (profile_id, limit),
                )
                rows = cursor.fetchall()
        entries = []
        for row in rows:
            entries.append(
                {
                    "created_at": row["created_at"],
                    "kind": row["kind"],
                    "trigger_source": row["trigger_source"],
                    "topic": row["topic"] or "",
                    "summary_snippet": row["summary_snippet"] or "",
                    "artifact_path": row["artifact_path"] or "",
                    "text_excerpt": (row["text"] or "").strip()[:4000],
                }
            )
        return entries

    def select_memory_context(
        self,
        kind: str,
        topic: str,
        trigger_source: str,
        recent_limit: int,
        relevant_limit: int,
    ) -> dict[str, object]:
        entries = self._memory_rows(limit=48)
        summary = build_summary_text(entries[:12], max_chars=1800) if entries else ""
        return build_memory_payload(entries, kind, topic, trigger_source, recent_limit, relevant_limit, summary.strip())

    def refresh_summary(self, summary_file: Path, entry_limit: int, max_chars: int) -> str:
        entries = self._memory_rows(limit=entry_limit)
        if not entries:
            text = "No HAL aware outputs have been recorded yet.\n"
        else:
            text = build_summary_text(entries, max_chars=max_chars)
        ensure_parent(summary_file)
        summary_file.write_text(text, encoding="utf-8")
        return text

    def record_aware_output(
        self,
        kind: str,
        topic: str,
        trigger_source: str,
        trigger_id: str,
        text: str,
        artifact_path: str,
    ) -> None:
        created_at = now_iso()
        summary_seed = f"{topic}\n{text}" if topic else text
        summary_snippet = summarize_text(summary_seed)
        metadata = {
            "trigger_id": trigger_id,
            "artifact_path": artifact_path,
            "legacy_source": "halo_state.record_aware_output",
        }
        with connect(self.config.postgres_dsn) as connection:
            profile_id = self._profile_id(connection)
            with connection.cursor() as cursor:
                cursor.execute(
                    f"""
                    INSERT INTO {self.schema}.generated_artifacts (
                        profile_id, artifact_type, title, topic, prompt_seed, summary, body,
                        file_path, audio_path, metadata_json, created_at, updated_at
                    ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s::jsonb, %s, %s)
                    RETURNING id
                    """,
                    (
                        profile_id,
                        kind,
                        summarize_text(topic, 120),
                        topic,
                        topic,
                        summary_snippet,
                        text,
                        artifact_path,
                        "",
                        json.dumps(metadata, sort_keys=True),
                        created_at,
                        created_at,
                    ),
                )
                artifact_id = cursor.fetchone()["id"]
                cursor.execute(
                    f"""
                    INSERT INTO {self.schema}.memory_events (
                        profile_id, event_type, event_source, topic, title, summary, body,
                        trigger_id, request_id, source_artifact_id, metadata_json, observed_at, created_at
                    ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s::jsonb, %s, %s)
                    RETURNING id
                    """,
                    (
                        profile_id,
                        kind,
                        trigger_source,
                        topic,
                        summarize_text(topic, 120),
                        summary_snippet,
                        text,
                        trigger_id,
                        f"aware:{trigger_source}:{trigger_id}:{created_at}",
                        artifact_id,
                        json.dumps(metadata, sort_keys=True),
                        created_at,
                        created_at,
                    ),
                )
                event_id = cursor.fetchone()["id"]
                cursor.execute(
                    f"UPDATE {self.schema}.generated_artifacts SET source_event_id = %s WHERE id = %s",
                    (event_id, artifact_id),
                )
            connection.commit()

    def evaluate_triggers(self, trigger_file: Path) -> dict[str, str] | None:
        if not trigger_file.exists():
            return None
        payload = json.loads(trigger_file.read_text(encoding="utf-8"))
        triggers = payload.get("triggers", [])
        with connect(self.config.postgres_dsn) as connection:
            profile_id = self._profile_id(connection)
            selected = None
            for trigger in triggers:
                if not trigger.get("enabled"):
                    continue
                trigger_type = trigger.get("type")
                trigger_id = trigger.get("id") or "unnamed-trigger"
                kind = trigger.get("kind") or "observation"
                topic = trigger.get("topic") or ""

                with connection.cursor() as cursor:
                    cursor.execute(
                        f"""
                        SELECT observed_at::text AS observed_at
                        FROM {self.schema}.memory_events
                        WHERE profile_id = %s AND trigger_id = %s
                        ORDER BY observed_at DESC
                        LIMIT 1
                        """,
                        (profile_id, trigger_id),
                    )
                    row = cursor.fetchone()

                last_time = None
                if row and row.get("observed_at"):
                    try:
                        from datetime import datetime

                        last_time = datetime.fromisoformat(row["observed_at"].replace("Z", "+00:00"))
                    except Exception:
                        last_time = None

                from datetime import datetime, timezone
                import random

                now = datetime.now(timezone.utc)
                if trigger_type == "interval":
                    interval_seconds = int(trigger.get("interval_seconds", 0) or 0)
                    if interval_seconds <= 0:
                        continue
                    if last_time is None or (now - last_time).total_seconds() >= interval_seconds:
                        selected = {
                            "kind": kind,
                            "topic": topic,
                            "trigger_source": "interval",
                            "trigger_id": trigger_id,
                        }
                        break
                if trigger_type == "random_chance":
                    chance = float(trigger.get("chance", 0) or 0)
                    if chance > 0 and random.random() <= chance:
                        selected = {
                            "kind": kind,
                            "topic": topic,
                            "trigger_source": "random_chance",
                            "trigger_id": trigger_id,
                        }
                        break
            return selected

