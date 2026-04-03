import datetime
import json
import os
from pathlib import Path

from ..models import Observation, SensorRunResult
from .base import Sensor


class BlinkSensor(Sensor):
    name = "blink"

    def collect(self) -> SensorRunResult:
        started_at = datetime.datetime.now(datetime.timezone.utc).isoformat()
        now = started_at
        metadata_dir = os.environ.get("HAL_BLINK_METADATA_DIR", "")
        if not metadata_dir:
            return SensorRunResult(
                sensor_name=self.name,
                started_at=started_at,
                finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                status="skipped",
                metadata={"reason": "HAL_BLINK_METADATA_DIR not configured"},
            )

        root = Path(metadata_dir)
        events = []
        for path in sorted(root.glob("*.json"))[:200]:
            try:
                events.append(json.loads(path.read_text(encoding="utf-8")))
            except Exception:
                continue

        return SensorRunResult(
            sensor_name=self.name,
            started_at=started_at,
            finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
            status="ok",
            observations=[
                Observation(
                    sensor_name=self.name,
                    observation_type="blink.motion.events",
                    subject_key="blink:exterior",
                    title="Blink metadata snapshot",
                    summary="Observed %d Blink motion metadata events." % len(events),
                    observed_at=now,
                    significance=0.38,
                    payload={"event_count": len(events), "sample": events[:5]},
                )
            ] if events else [],
            metadata={"event_count": len(events)},
        )