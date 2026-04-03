import json
import sqlite3
from pathlib import Path
from typing import Dict, List

from .models import KnowledgeEntity, KnowledgeFact, KnowledgeRelationship, Observation, SensorRunResult


class KnowledgeStore:
    def __init__(self, db_path: Path) -> None:
        self.db_path = db_path
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        self._initialize()

    def _connect(self) -> sqlite3.Connection:
        connection = sqlite3.connect(str(self.db_path))
        connection.row_factory = sqlite3.Row
        connection.execute("PRAGMA journal_mode = WAL")
        connection.execute("PRAGMA busy_timeout = 5000")
        return connection

    def _initialize(self) -> None:
        connection = self._connect()
        try:
            connection.executescript(
                """
                CREATE TABLE IF NOT EXISTS sensor_runs (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    sensor_name TEXT NOT NULL,
                    started_at TEXT NOT NULL,
                    finished_at TEXT NOT NULL,
                    status TEXT NOT NULL,
                    metadata_json TEXT NOT NULL,
                    errors_json TEXT NOT NULL,
                    commentary_hint TEXT
                );

                CREATE TABLE IF NOT EXISTS observations (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    sensor_name TEXT NOT NULL,
                    observation_type TEXT NOT NULL,
                    subject_key TEXT NOT NULL,
                    title TEXT NOT NULL,
                    summary TEXT NOT NULL,
                    observed_at TEXT NOT NULL,
                    significance REAL NOT NULL,
                    payload_json TEXT NOT NULL
                );

                CREATE TABLE IF NOT EXISTS knowledge_entities (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    entity_type TEXT NOT NULL,
                    entity_key TEXT NOT NULL UNIQUE,
                    display_name TEXT NOT NULL,
                    first_seen TEXT NOT NULL,
                    last_seen TEXT NOT NULL,
                    attributes_json TEXT NOT NULL
                );

                CREATE TABLE IF NOT EXISTS knowledge_relationships (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    relationship_type TEXT NOT NULL,
                    source_entity_key TEXT NOT NULL,
                    target_entity_key TEXT NOT NULL,
                    first_seen TEXT NOT NULL,
                    last_seen TEXT NOT NULL,
                    strength REAL NOT NULL,
                    attributes_json TEXT NOT NULL,
                    UNIQUE(relationship_type, source_entity_key, target_entity_key)
                );

                CREATE TABLE IF NOT EXISTS knowledge_facts (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    sensor_name TEXT NOT NULL,
                    fact_type TEXT NOT NULL,
                    subject_key TEXT NOT NULL,
                    fact_key TEXT NOT NULL,
                    observed_at TEXT NOT NULL,
                    summary TEXT NOT NULL,
                    confidence REAL NOT NULL,
                    payload_json TEXT NOT NULL,
                    UNIQUE(sensor_name, fact_type, subject_key, fact_key)
                );

                CREATE TABLE IF NOT EXISTS commentary_history (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    trigger_key TEXT NOT NULL,
                    fingerprint TEXT NOT NULL,
                    summary TEXT NOT NULL,
                    emitted_at TEXT NOT NULL,
                    metadata_json TEXT NOT NULL
                );

                CREATE INDEX IF NOT EXISTS idx_sensor_runs_name_time ON sensor_runs(sensor_name, finished_at DESC);
                CREATE INDEX IF NOT EXISTS idx_observations_sensor_time ON observations(sensor_name, observed_at DESC);
                CREATE INDEX IF NOT EXISTS idx_observations_subject_time ON observations(subject_key, observed_at DESC);
                CREATE INDEX IF NOT EXISTS idx_facts_subject_type ON knowledge_facts(subject_key, fact_type, observed_at DESC);
                CREATE INDEX IF NOT EXISTS idx_commentary_history_trigger_time ON commentary_history(trigger_key, emitted_at DESC);
                """
            )
            connection.commit()
        finally:
            connection.close()

    def record_run(self, result: SensorRunResult) -> None:
        connection = self._connect()
        try:
            connection.execute(
                """
                INSERT INTO sensor_runs (sensor_name, started_at, finished_at, status, metadata_json, errors_json, commentary_hint)
                VALUES (?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    result.sensor_name,
                    result.started_at,
                    result.finished_at,
                    result.status,
                    json.dumps(result.metadata, sort_keys=True),
                    json.dumps(result.errors),
                    result.commentary_hint,
                ),
            )

            for observation in result.observations:
                self._insert_observation(connection, observation)

            for entity in result.entities:
                self._upsert_entity(connection, entity)

            for relationship in result.relationships:
                self._upsert_relationship(connection, relationship)

            for fact in result.facts:
                self._upsert_fact(connection, fact)

            connection.commit()
        finally:
            connection.close()

    def _insert_observation(self, connection: sqlite3.Connection, observation: Observation) -> None:
        connection.execute(
            """
            INSERT INTO observations (sensor_name, observation_type, subject_key, title, summary, observed_at, significance, payload_json)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                observation.sensor_name,
                observation.observation_type,
                observation.subject_key,
                observation.title,
                observation.summary,
                observation.observed_at,
                observation.significance,
                json.dumps(observation.payload, sort_keys=True),
            ),
        )

    def _upsert_entity(self, connection: sqlite3.Connection, entity: KnowledgeEntity) -> None:
        connection.execute(
            """
            INSERT INTO knowledge_entities (entity_type, entity_key, display_name, first_seen, last_seen, attributes_json)
            VALUES (?, ?, ?, ?, ?, ?)
            ON CONFLICT(entity_key) DO UPDATE SET
                display_name = excluded.display_name,
                last_seen = excluded.last_seen,
                attributes_json = excluded.attributes_json
            """,
            (
                entity.entity_type,
                entity.entity_key,
                entity.display_name,
                entity.first_seen,
                entity.last_seen,
                json.dumps(entity.attributes, sort_keys=True),
            ),
        )

    def _upsert_relationship(self, connection: sqlite3.Connection, relationship: KnowledgeRelationship) -> None:
        connection.execute(
            """
            INSERT INTO knowledge_relationships (
                relationship_type, source_entity_key, target_entity_key, first_seen, last_seen, strength, attributes_json
            ) VALUES (?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(relationship_type, source_entity_key, target_entity_key) DO UPDATE SET
                last_seen = excluded.last_seen,
                strength = excluded.strength,
                attributes_json = excluded.attributes_json
            """,
            (
                relationship.relationship_type,
                relationship.source_entity_key,
                relationship.target_entity_key,
                relationship.first_seen,
                relationship.last_seen,
                relationship.strength,
                json.dumps(relationship.attributes, sort_keys=True),
            ),
        )

    def _upsert_fact(self, connection: sqlite3.Connection, fact: KnowledgeFact) -> None:
        connection.execute(
            """
            INSERT INTO knowledge_facts (sensor_name, fact_type, subject_key, fact_key, observed_at, summary, confidence, payload_json)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(sensor_name, fact_type, subject_key, fact_key) DO UPDATE SET
                observed_at = excluded.observed_at,
                summary = excluded.summary,
                confidence = excluded.confidence,
                payload_json = excluded.payload_json
            """,
            (
                fact.sensor_name,
                fact.fact_type,
                fact.subject_key,
                fact.fact_key,
                fact.observed_at,
                fact.summary,
                fact.confidence,
                json.dumps(fact.payload, sort_keys=True),
            ),
        )

    def status_snapshot(self) -> Dict[str, object]:
        connection = self._connect()
        try:
            sensors = connection.execute(
                "SELECT sensor_name, status, finished_at FROM sensor_runs ORDER BY finished_at DESC"
            ).fetchall()
            latest_by_sensor = {}
            for row in sensors:
                if row["sensor_name"] not in latest_by_sensor:
                    latest_by_sensor[row["sensor_name"]] = {
                        "status": row["status"],
                        "finished_at": row["finished_at"],
                    }

            return {
                "sensor_runs": latest_by_sensor,
                "observation_count": connection.execute("SELECT COUNT(*) FROM observations").fetchone()[0],
                "entity_count": connection.execute("SELECT COUNT(*) FROM knowledge_entities").fetchone()[0],
                "relationship_count": connection.execute("SELECT COUNT(*) FROM knowledge_relationships").fetchone()[0],
                "fact_count": connection.execute("SELECT COUNT(*) FROM knowledge_facts").fetchone()[0],
            }
        finally:
            connection.close()

    def latest_observations(self, limit: int = 10) -> List[Dict[str, object]]:
        connection = self._connect()
        try:
            rows = connection.execute(
                "SELECT sensor_name, observation_type, subject_key, title, summary, observed_at, significance FROM observations ORDER BY observed_at DESC LIMIT ?",
                (limit,),
            ).fetchall()
            return [dict(row) for row in rows]
        finally:
            connection.close()

    def commentary_allowed(self, trigger_key: str, fingerprint: str, cooldown_seconds: int, now_iso: str) -> bool:
        connection = self._connect()
        try:
            row = connection.execute(
                "SELECT emitted_at, fingerprint FROM commentary_history WHERE trigger_key = ? ORDER BY emitted_at DESC LIMIT 1",
                (trigger_key,),
            ).fetchone()
            if row is None:
                return True

            from datetime import datetime, timedelta, timezone

            last_emitted = datetime.fromisoformat(row["emitted_at"].replace("Z", "+00:00"))
            current = datetime.fromisoformat(now_iso.replace("Z", "+00:00"))
            if current - last_emitted < timedelta(seconds=cooldown_seconds):
                return False
            return True
        finally:
            connection.close()

    def record_commentary_emission(self, trigger_key: str, fingerprint: str, summary: str, emitted_at: str, metadata: Dict[str, object]) -> None:
        connection = self._connect()
        try:
            connection.execute(
                "INSERT INTO commentary_history (trigger_key, fingerprint, summary, emitted_at, metadata_json) VALUES (?, ?, ?, ?, ?)",
                (trigger_key, fingerprint, summary, emitted_at, json.dumps(metadata, sort_keys=True)),
            )
            connection.commit()
        finally:
            connection.close()