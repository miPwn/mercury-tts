import datetime
import hashlib
from typing import Dict, Iterable, List, Optional

from .models import SensorRunResult


class PerceptionPipeline:
    def summarize(self, results: Iterable[SensorRunResult]) -> List[str]:
        lines = []
        timestamp = datetime.datetime.now(datetime.timezone.utc).isoformat()
        for result in results:
            if result.commentary_hint:
                lines.append("[%s] %s" % (timestamp, result.commentary_hint))
            elif result.observations:
                lines.append("[%s] %s" % (timestamp, result.observations[0].summary))
        return lines

    def build_commentary_candidate(self, results: Iterable[SensorRunResult], threshold: float = 0.55) -> Optional[Dict[str, object]]:
        selected = []
        for result in results:
            for observation in result.observations:
                if observation.significance >= threshold:
                    selected.append(observation)

        if not selected:
            return None

        selected.sort(key=lambda item: item.significance, reverse=True)
        top = selected[:3]
        summary = " ".join(observation.summary for observation in top)
        topic = "; ".join("%s: %s" % (observation.sensor_name, observation.summary) for observation in top)
        fingerprint = hashlib.sha256(topic.encode("utf-8")).hexdigest()
        return {
            "summary": summary,
            "topic": topic,
            "fingerprint": fingerprint,
            "observation_count": len(top),
            "max_significance": top[0].significance,
        }