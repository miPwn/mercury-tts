from pathlib import Path
import datetime
from typing import Dict, List

from .knowledge_store import KnowledgeStore
from .observation_log import ObservationLog
from .perception import PerceptionPipeline
from .sensors.blink import BlinkSensor
from .sensors.host import HostSensor
from .sensors.network import NetworkSensor
from .sensors.photoprism import PhotoPrismSensor


class SensorManager:
    def __init__(self, halo_root: Path) -> None:
        self.halo_root = halo_root
        self.state_dir = halo_root / "state" / "halo" / "sensory"
        self.store = KnowledgeStore()
        self.log = ObservationLog(self.state_dir / "observation-log.jsonl")
        self.pipeline = PerceptionPipeline()
        self.sensors = {
            "host": HostSensor(halo_root),
            "network": NetworkSensor(halo_root),
            "photoprism": PhotoPrismSensor(halo_root),
            "blink": BlinkSensor(halo_root),
        }

    def available_sensors(self) -> List[str]:
        return sorted(self.sensors.keys())

    def run(self, sensor_name: str = "all") -> List[dict]:
        selected = self._select(sensor_name)
        results = []
        for sensor in selected:
            result = sensor.collect()
            self.store.record_run(result)
            for observation in result.observations:
                self.log.append(
                    {
                        "sensor_name": observation.sensor_name,
                        "observation_type": observation.observation_type,
                        "subject_key": observation.subject_key,
                        "title": observation.title,
                        "summary": observation.summary,
                        "observed_at": observation.observed_at,
                        "significance": observation.significance,
                    }
                )
            results.append(result)

        commentary = self.pipeline.summarize(results)
        return [self._render_result(result, commentary) for result in results]

    def status(self) -> Dict[str, object]:
        snapshot = self.store.status_snapshot()
        snapshot["available_sensors"] = self.available_sensors()
        snapshot["recent_observations"] = self.store.latest_observations(8)
        return snapshot

    def commentary_candidate(self, sensor_name: str = "all", threshold: float = 0.55, cooldown_seconds: int = 1800) -> Dict[str, object]:
        selected = self._select(sensor_name)
        results = []
        for sensor in selected:
            result = sensor.collect()
            self.store.record_run(result)
            for observation in result.observations:
                self.log.append(
                    {
                        "sensor_name": observation.sensor_name,
                        "observation_type": observation.observation_type,
                        "subject_key": observation.subject_key,
                        "title": observation.title,
                        "summary": observation.summary,
                        "observed_at": observation.observed_at,
                        "significance": observation.significance,
                    }
                )
            results.append(result)

        candidate = self.pipeline.build_commentary_candidate(results, threshold)
        now = datetime.datetime.now(datetime.timezone.utc).isoformat()
        if candidate is None:
            return {
                "sensor_name": sensor_name,
                "should_generate": False,
                "reason": "no-significant-observations",
                "results": [self._render_result(result, []) for result in results],
            }

        trigger_key = "sensory:%s" % sensor_name
        allowed = self.store.commentary_allowed(trigger_key, candidate["fingerprint"], cooldown_seconds, now)
        return {
            "sensor_name": sensor_name,
            "should_generate": allowed,
            "reason": "ok" if allowed else "cooldown",
            "trigger_key": trigger_key,
            "cooldown_seconds": cooldown_seconds,
            "candidate": candidate,
            "results": [self._render_result(result, [candidate["summary"]]) for result in results],
        }

    def mark_commentary_emitted(self, trigger_key: str, fingerprint: str, summary: str, metadata: Dict[str, object]) -> None:
        emitted_at = datetime.datetime.now(datetime.timezone.utc).isoformat()
        self.store.record_commentary_emission(trigger_key, fingerprint, summary, emitted_at, metadata)

    def _select(self, sensor_name: str):
        if sensor_name == "all":
            return [self.sensors[name] for name in self.available_sensors()]
        if sensor_name not in self.sensors:
            raise KeyError("Unknown sensor: %s" % sensor_name)
        return [self.sensors[sensor_name]]

    def _render_result(self, result, commentary):
        return {
            "sensor_name": result.sensor_name,
            "status": result.status,
            "observation_count": len(result.observations),
            "entity_count": len(result.entities),
            "relationship_count": len(result.relationships),
            "fact_count": len(result.facts),
            "metadata": result.metadata,
            "errors": result.errors,
            "commentary_hint": result.commentary_hint,
            "generated_commentary_candidates": commentary,
        }
