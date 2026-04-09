import json
import re
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, List, Optional

from halo_state.config import StateConfig
from halo_state.postgres import connect
from .models import KnowledgeEntity, KnowledgeFact, KnowledgeRelationship, Observation, SensorRunResult


_IDENTIFIER_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


def _safe_schema(value: str) -> str:
    schema = (value or "halo").strip()
    if not _IDENTIFIER_RE.fullmatch(schema):
        raise RuntimeError(f"Invalid HALO_STATE_POSTGRES_SCHEMA: {value!r}")
    return schema


def _as_datetime(value: object) -> datetime:
    if isinstance(value, datetime):
        return value
    if isinstance(value, str):
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    raise RuntimeError(f"Unsupported timestamp value type: {type(value)!r}")


class KnowledgeStore:
    def __init__(self, _legacy_db_path: Optional[Path] = None) -> None:
        self.config = StateConfig.from_env()
        if not self.config.postgres_dsn:
            raise RuntimeError("HALO_STATE_POSTGRES_DSN is required for sensory state persistence.")
        self.schema = _safe_schema(self.config.postgres_schema)
        self.profile_key = self.config.profile_key

    def _profile_id(self, connection) -> str:
        with connection.cursor() as cursor:
            cursor.execute(
                f"SELECT id FROM {self.schema}.identity_profiles WHERE profile_key = %s",
                (self.profile_key,),
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
                    self.profile_key,
                    self.profile_key.upper(),
                    "Auto-created HAL identity profile for sensory pipeline.",
                    "",
                ),
            )
            created = cursor.fetchone()
            connection.commit()
            return created["id"]

    def record_run(self, result: SensorRunResult) -> None:
        with connect(self.config.postgres_dsn) as connection:
            profile_id = self._profile_id(connection)
            with connection.cursor() as cursor:
                cursor.execute(
                    f"""
                    INSERT INTO {self.schema}.sensor_runs (
                        sensor_name, started_at, finished_at, status, metadata_json, errors_json, commentary_hint
                    ) VALUES (%s, %s, %s, %s, %s::jsonb, %s::jsonb, %s)
                    ON CONFLICT (sensor_name, started_at, finished_at) DO UPDATE SET
                        status = EXCLUDED.status,
                        metadata_json = EXCLUDED.metadata_json,
                        errors_json = EXCLUDED.errors_json,
                        commentary_hint = EXCLUDED.commentary_hint
                    RETURNING id
                    """,
                    (
                        result.sensor_name,
                        result.started_at,
                        result.finished_at,
                        result.status,
                        json.dumps(result.metadata, sort_keys=True),
                        json.dumps(result.errors),
                        result.commentary_hint or "",
                    ),
                )
                sensor_run_id = cursor.fetchone()["id"]

                for observation in result.observations:
                    self._insert_observation(cursor, sensor_run_id, observation)

                for entity in result.entities:
                    self._upsert_entity(cursor, entity)

                for relationship in result.relationships:
                    self._upsert_relationship(cursor, relationship)

                for fact in result.facts:
                    self._upsert_fact(cursor, profile_id, fact)
            connection.commit()

    def _insert_observation(self, cursor, sensor_run_id: str, observation: Observation) -> None:
        cursor.execute(
            f"""
            INSERT INTO {self.schema}.observations (
                sensor_run_id, sensor_name, observation_type, subject_key, title, summary, observed_at, significance, payload_json
            ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s::jsonb)
            ON CONFLICT (sensor_name, observation_type, subject_key, observed_at, title) DO UPDATE SET
                sensor_run_id = EXCLUDED.sensor_run_id,
                summary = EXCLUDED.summary,
                significance = EXCLUDED.significance,
                payload_json = EXCLUDED.payload_json
            """,
            (
                sensor_run_id,
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

    def _upsert_entity(self, cursor, entity: KnowledgeEntity) -> None:
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
                entity.entity_type,
                entity.entity_key,
                entity.display_name,
                entity.first_seen,
                entity.last_seen,
                json.dumps(entity.attributes, sort_keys=True),
            ),
        )

    def _upsert_relationship(self, cursor, relationship: KnowledgeRelationship) -> None:
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
                relationship.relationship_type,
                relationship.source_entity_key,
                relationship.target_entity_key,
                relationship.first_seen,
                relationship.last_seen,
                relationship.strength,
                json.dumps(relationship.attributes, sort_keys=True),
            ),
        )

    def _upsert_fact(self, cursor, profile_id: str, fact: KnowledgeFact) -> None:
        cursor.execute(
            f"""
            INSERT INTO {self.schema}.knowledge_facts (
                profile_id, fact_key, fact_type, subject_key, summary, payload_json, confidence, source_kind, source_ref, observed_at
            ) VALUES (%s, %s, %s, %s, %s, %s::jsonb, %s, %s, %s, %s)
            ON CONFLICT (fact_type, subject_key, fact_key, source_kind, source_ref) DO UPDATE SET
                observed_at = EXCLUDED.observed_at,
                summary = EXCLUDED.summary,
                confidence = EXCLUDED.confidence,
                payload_json = EXCLUDED.payload_json,
                profile_id = EXCLUDED.profile_id
            """,
            (
                profile_id,
                fact.fact_key,
                fact.fact_type,
                fact.subject_key,
                fact.summary,
                json.dumps(fact.payload, sort_keys=True),
                fact.confidence,
                "sensory",
                fact.sensor_name,
                fact.observed_at,
            ),
        )

    def status_snapshot(self) -> Dict[str, object]:
        with connect(self.config.postgres_dsn) as connection:
            with connection.cursor() as cursor:
                cursor.execute(
                    f"""
                    SELECT sensor_name, status, finished_at::text AS finished_at
                    FROM {self.schema}.sensor_runs
                    ORDER BY finished_at DESC
                    """
                )
                sensors = cursor.fetchall()
                latest_by_sensor = {}
                for row in sensors:
                    if row["sensor_name"] not in latest_by_sensor:
                        latest_by_sensor[row["sensor_name"]] = {
                            "status": row["status"],
                            "finished_at": row["finished_at"],
                        }

                cursor.execute(f"SELECT COUNT(*) AS value FROM {self.schema}.observations")
                observation_count = int(cursor.fetchone()["value"])
                cursor.execute(f"SELECT COUNT(*) AS value FROM {self.schema}.knowledge_entities")
                entity_count = int(cursor.fetchone()["value"])
                cursor.execute(f"SELECT COUNT(*) AS value FROM {self.schema}.knowledge_relationships")
                relationship_count = int(cursor.fetchone()["value"])
                cursor.execute(f"SELECT COUNT(*) AS value FROM {self.schema}.knowledge_facts")
                fact_count = int(cursor.fetchone()["value"])

        return {
            "sensor_runs": latest_by_sensor,
            "observation_count": observation_count,
            "entity_count": entity_count,
            "relationship_count": relationship_count,
            "fact_count": fact_count,
        }

    def latest_observations(self, limit: int = 10) -> List[Dict[str, object]]:
        with connect(self.config.postgres_dsn) as connection:
            with connection.cursor() as cursor:
                cursor.execute(
                    f"""
                    SELECT sensor_name, observation_type, subject_key, title, summary, observed_at::text AS observed_at, significance
                    FROM {self.schema}.observations
                    ORDER BY observed_at DESC
                    LIMIT %s
                    """,
                    (max(0, int(limit)),),
                )
                rows = cursor.fetchall()
                return [dict(row) for row in rows]

    def commentary_allowed(self, trigger_key: str, fingerprint: str, cooldown_seconds: int, now_iso: str) -> bool:
        with connect(self.config.postgres_dsn) as connection:
            with connection.cursor() as cursor:
                cursor.execute(
                    f"""
                    SELECT emitted_at
                    FROM {self.schema}.commentary_history
                    WHERE trigger_key = %s
                    ORDER BY emitted_at DESC
                    LIMIT 1
                    """,
                    (trigger_key,),
                )
                row = cursor.fetchone()
                if row is None:
                    return True
                last_emitted = _as_datetime(row["emitted_at"])

        current = datetime.fromisoformat(now_iso.replace("Z", "+00:00"))
        return current - last_emitted >= timedelta(seconds=cooldown_seconds)

    def record_commentary_emission(
        self,
        trigger_key: str,
        fingerprint: str,
        summary: str,
        emitted_at: str,
        metadata: Dict[str, object],
    ) -> None:
        with connect(self.config.postgres_dsn) as connection:
            with connection.cursor() as cursor:
                cursor.execute(
                    f"""
                    INSERT INTO {self.schema}.commentary_history (
                        trigger_key, fingerprint, summary, emitted_at, metadata_json
                    ) VALUES (%s, %s, %s, %s, %s::jsonb)
                    ON CONFLICT (trigger_key, fingerprint, emitted_at) DO NOTHING
                    """,
                    (
                        trigger_key,
                        fingerprint,
                        summary,
                        emitted_at,
                        json.dumps(metadata, sort_keys=True),
                    ),
                )
            connection.commit()
