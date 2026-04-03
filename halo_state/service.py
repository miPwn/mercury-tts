from __future__ import annotations

import json
import sqlite3
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
                    "text_excerpt": (row["text"] or "").strip()[:700],
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
        summary_snippet = summarize_text(text)
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

    def _aware_exists(self, connection, request_id: str) -> bool:
        with connection.cursor() as cursor:
            cursor.execute(
                f"SELECT 1 FROM {self.schema}.memory_events WHERE request_id = %s LIMIT 1",
                (request_id,),
            )
            return cursor.fetchone() is not None

    def migrate_legacy_state(
        self,
        aware_db: Path | None,
        sensory_db: Path | None,
        commentary_db: Path | None,
    ) -> dict[str, int]:
        stats = {
            "aware_imported": 0,
            "aware_skipped": 0,
            "sensor_runs_imported": 0,
            "observations_imported": 0,
            "knowledge_entities_imported": 0,
            "knowledge_relationships_imported": 0,
            "knowledge_facts_imported": 0,
            "commentary_history_imported": 0,
            "commentary_cycles_imported": 0,
            "commentary_line_history_imported": 0,
        }
        with connect(self.config.postgres_dsn) as connection:
            profile_id = self._profile_id(connection)

            if aware_db and aware_db.exists():
                aware = sqlite3.connect(str(aware_db))
                aware.row_factory = sqlite3.Row
                try:
                    rows = aware.execute(
                        "SELECT created_at, kind, trigger_source, trigger_id, topic, text, summary_snippet, content_hash, artifact_path, metadata_json FROM aware_outputs ORDER BY id"
                    ).fetchall()
                except sqlite3.Error:
                    rows = []
                finally:
                    aware.close()

                for row in rows:
                    request_id = f"legacy-aware:{row['content_hash']}:{row['kind']}:{row['trigger_id']}"
                    if self._aware_exists(connection, request_id):
                        stats["aware_skipped"] += 1
                        continue
                    metadata = {}
                    if row["metadata_json"]:
                        try:
                            metadata = json.loads(row["metadata_json"])
                        except Exception:
                            metadata = {}
                    metadata["legacy_content_hash"] = row["content_hash"]
                    metadata["legacy_import"] = True
                    with connection.cursor() as cursor:
                        cursor.execute(
                            f"""
                            INSERT INTO {self.schema}.generated_artifacts (
                                profile_id, artifact_type, title, topic, prompt_seed, summary, body,
                                file_path, metadata_json, created_at, updated_at
                            ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s::jsonb, %s, %s)
                            RETURNING id
                            """,
                            (
                                profile_id,
                                row["kind"],
                                summarize_text(row["topic"] or "", 120),
                                row["topic"] or "",
                                row["topic"] or "",
                                row["summary_snippet"] or summarize_text(row["text"] or ""),
                                row["text"] or "",
                                row["artifact_path"] or "",
                                json.dumps(metadata, sort_keys=True),
                                row["created_at"],
                                row["created_at"],
                            ),
                        )
                        artifact_id = cursor.fetchone()["id"]
                        cursor.execute(
                            f"""
                            INSERT INTO {self.schema}.memory_events (
                                profile_id, event_type, event_source, topic, title, summary, body,
                                trigger_id, request_id, source_artifact_id, metadata_json, observed_at, created_at
                            ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s::jsonb, %s, %s)
                            """,
                            (
                                profile_id,
                                row["kind"],
                                row["trigger_source"],
                                row["topic"] or "",
                                summarize_text(row["topic"] or "", 120),
                                row["summary_snippet"] or summarize_text(row["text"] or ""),
                                row["text"] or "",
                                row["trigger_id"] or "",
                                request_id,
                                artifact_id,
                                json.dumps(metadata, sort_keys=True),
                                row["created_at"],
                                row["created_at"],
                            ),
                        )
                    stats["aware_imported"] += 1

            if sensory_db and sensory_db.exists():
                sensory = sqlite3.connect(str(sensory_db))
                sensory.row_factory = sqlite3.Row
                table_queries = {
                    "sensor_runs": "SELECT sensor_name, started_at, finished_at, status, metadata_json, errors_json, commentary_hint FROM sensor_runs",
                    "observations": "SELECT sensor_name, observation_type, subject_key, title, summary, observed_at, significance, payload_json FROM observations",
                    "knowledge_entities": "SELECT entity_type, entity_key, display_name, first_seen, last_seen, attributes_json FROM knowledge_entities",
                    "knowledge_relationships": "SELECT relationship_type, source_entity_key, target_entity_key, first_seen, last_seen, strength, attributes_json FROM knowledge_relationships",
                    "knowledge_facts": "SELECT sensor_name, fact_type, subject_key, fact_key, observed_at, summary, confidence, payload_json FROM knowledge_facts",
                    "commentary_history": "SELECT trigger_key, fingerprint, summary, emitted_at, metadata_json FROM commentary_history",
                }
                loaded: dict[str, list[sqlite3.Row]] = {}
                for key, query in table_queries.items():
                    try:
                        loaded[key] = sensory.execute(query).fetchall()
                    except sqlite3.Error:
                        loaded[key] = []
                sensory.close()

                with connection.cursor() as cursor:
                    for row in loaded["sensor_runs"]:
                        cursor.execute(
                            f"""
                            INSERT INTO {self.schema}.sensor_runs (
                                sensor_name, status, commentary_hint, metadata_json, errors_json, started_at, finished_at
                            ) VALUES (%s, %s, %s, %s::jsonb, %s::jsonb, %s, %s)
                            ON CONFLICT (sensor_name, started_at, finished_at) DO NOTHING
                            """,
                            (
                                row["sensor_name"],
                                row["status"],
                                row["commentary_hint"] or "",
                                row["metadata_json"] or "{}",
                                row["errors_json"] or "[]",
                                row["started_at"],
                                row["finished_at"],
                            ),
                        )
                        stats["sensor_runs_imported"] += 1

                    for row in loaded["observations"]:
                        cursor.execute(
                            f"""
                            INSERT INTO {self.schema}.observations (
                                sensor_name, observation_type, subject_key, title, summary, payload_json, significance, observed_at
                            ) VALUES (%s, %s, %s, %s, %s, %s::jsonb, %s, %s)
                            ON CONFLICT (sensor_name, observation_type, subject_key, observed_at, title) DO NOTHING
                            """,
                            (
                                row["sensor_name"],
                                row["observation_type"],
                                row["subject_key"],
                                row["title"],
                                row["summary"],
                                row["payload_json"] or "{}",
                                row["significance"],
                                row["observed_at"],
                            ),
                        )
                        stats["observations_imported"] += 1

                    for row in loaded["knowledge_entities"]:
                        cursor.execute(
                            f"""
                            INSERT INTO {self.schema}.knowledge_entities (
                                entity_type, entity_key, display_name, first_seen, last_seen, attributes_json
                            ) VALUES (%s, %s, %s, %s, %s, %s::jsonb)
                            ON CONFLICT (entity_key) DO UPDATE SET
                                display_name = EXCLUDED.display_name,
                                last_seen = EXCLUDED.last_seen,
                                attributes_json = EXCLUDED.attributes_json
                            """,
                            (
                                row["entity_type"],
                                row["entity_key"],
                                row["display_name"],
                                row["first_seen"],
                                row["last_seen"],
                                row["attributes_json"] or "{}",
                            ),
                        )
                        stats["knowledge_entities_imported"] += 1

                    for row in loaded["knowledge_relationships"]:
                        cursor.execute(
                            f"""
                            INSERT INTO {self.schema}.knowledge_relationships (
                                relationship_type, source_entity_key, target_entity_key, first_seen, last_seen, strength, attributes_json
                            ) VALUES (%s, %s, %s, %s, %s, %s, %s::jsonb)
                            ON CONFLICT (relationship_type, source_entity_key, target_entity_key) DO UPDATE SET
                                last_seen = EXCLUDED.last_seen,
                                strength = EXCLUDED.strength,
                                attributes_json = EXCLUDED.attributes_json
                            """,
                            (
                                row["relationship_type"],
                                row["source_entity_key"],
                                row["target_entity_key"],
                                row["first_seen"],
                                row["last_seen"],
                                row["strength"],
                                row["attributes_json"] or "{}",
                            ),
                        )
                        stats["knowledge_relationships_imported"] += 1

                    for row in loaded["knowledge_facts"]:
                        cursor.execute(
                            f"""
                            INSERT INTO {self.schema}.knowledge_facts (
                                fact_key, fact_type, subject_key, summary, payload_json, confidence, source_kind, source_ref, observed_at
                            ) VALUES (%s, %s, %s, %s, %s::jsonb, %s, %s, %s, %s)
                            ON CONFLICT (fact_type, subject_key, fact_key, source_kind, source_ref) DO UPDATE SET
                                observed_at = EXCLUDED.observed_at,
                                summary = EXCLUDED.summary,
                                confidence = EXCLUDED.confidence,
                                payload_json = EXCLUDED.payload_json
                            """,
                            (
                                row["fact_key"],
                                row["fact_type"],
                                row["subject_key"],
                                row["summary"],
                                row["payload_json"] or "{}",
                                row["confidence"],
                                "sensory",
                                row["sensor_name"],
                                row["observed_at"],
                            ),
                        )
                        stats["knowledge_facts_imported"] += 1

                    for row in loaded["commentary_history"]:
                        cursor.execute(
                            f"""
                            INSERT INTO {self.schema}.commentary_history (
                                trigger_key, fingerprint, summary, emitted_at, metadata_json
                            ) VALUES (%s, %s, %s, %s, %s::jsonb)
                            ON CONFLICT (trigger_key, fingerprint, emitted_at) DO NOTHING
                            """,
                            (
                                row["trigger_key"],
                                row["fingerprint"],
                                row["summary"],
                                row["emitted_at"],
                                row["metadata_json"] or "{}",
                            ),
                        )
                        stats["commentary_history_imported"] += 1

            if commentary_db and commentary_db.exists():
                commentary = sqlite3.connect(str(commentary_db))
                commentary.row_factory = sqlite3.Row
                try:
                    cycles = commentary.execute(
                        "SELECT commentary_file, current_cycle, updated_at FROM commentary_cycles"
                    ).fetchall()
                except sqlite3.Error:
                    cycles = []
                try:
                    rows = commentary.execute(
                        "SELECT commentary_file, line_hash, line_text, cycle, played_at FROM commentary_history ORDER BY id"
                    ).fetchall()
                except sqlite3.Error:
                    rows = []
                commentary.close()

                with connection.cursor() as cursor:
                    for row in cycles:
                        cursor.execute(
                            f"""
                            INSERT INTO {self.schema}.commentary_cycles (
                                commentary_file, current_cycle, updated_at
                            ) VALUES (%s, %s, %s)
                            ON CONFLICT (commentary_file) DO UPDATE SET
                                current_cycle = EXCLUDED.current_cycle,
                                updated_at = EXCLUDED.updated_at
                            """,
                            (
                                row["commentary_file"],
                                row["current_cycle"],
                                row["updated_at"],
                            ),
                        )
                        stats["commentary_cycles_imported"] += 1

                    for row in rows:
                        cursor.execute(
                            f"""
                            INSERT INTO {self.schema}.commentary_line_history (
                                commentary_file, line_hash, line_text, cycle, played_at
                            ) VALUES (%s, %s, %s, %s, %s)
                            ON CONFLICT (commentary_file, line_hash, cycle) DO NOTHING
                            """,
                            (
                                row["commentary_file"],
                                row["line_hash"],
                                row["line_text"],
                                row["cycle"],
                                row["played_at"],
                            ),
                        )
                        stats["commentary_line_history_imported"] += 1
                connection.commit()
        return stats
