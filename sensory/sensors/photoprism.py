import datetime
import json
import os
from collections import Counter, defaultdict
from urllib.error import HTTPError
from urllib.parse import urlencode
from urllib.request import Request, urlopen

from ..models import KnowledgeEntity, KnowledgeFact, KnowledgeRelationship, Observation, SensorRunResult
from .base import Sensor


class PhotoPrismSensor(Sensor):
    name = "photoprism"

    def collect(self) -> SensorRunResult:
        started_at = datetime.datetime.now(datetime.timezone.utc).isoformat()
        now = started_at
        base_url = os.environ.get("HAL_PHOTOPRISM_URL", "").rstrip("/")
        api_key = os.environ.get("HAL_PHOTOPRISM_API_KEY", "")

        if not base_url or not api_key:
            return SensorRunResult(
                sensor_name=self.name,
                started_at=started_at,
                finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                status="skipped",
                metadata={"reason": "HAL_PHOTOPRISM_URL or HAL_PHOTOPRISM_API_KEY not configured"},
            )

        photos = self._fetch_photos(base_url, api_key)
        people_counter = Counter()
        pair_counter = Counter()
        location_counter = Counter()
        person_locations = defaultdict(Counter)
        person_months = defaultdict(Counter)
        entities = []
        relationships = []
        facts = []

        for photo in photos:
            people = sorted(set(photo.get("people", [])))
            location = photo.get("location") or ""
            month_key = (photo.get("taken_at") or "")[:7]
            for person in people:
                people_counter[person] += 1
                if location:
                    person_locations[person][location] += 1
                if month_key:
                    person_months[person][month_key] += 1
            for index, left in enumerate(people):
                for right in people[index + 1 :]:
                    pair_counter[(left, right)] += 1
            if location:
                location_counter[location] += 1

        for person, count in people_counter.items():
            top_locations = [{"location": name, "count": seen} for name, seen in person_locations[person].most_common(5)]
            active_months = [{"month": name, "count": seen} for name, seen in person_months[person].most_common(6)]
            entities.append(
                KnowledgeEntity(
                    entity_type="person",
                    entity_key="person:%s" % person.lower().replace(" ", "-"),
                    display_name=person,
                    first_seen=now,
                    last_seen=now,
                    attributes={
                        "appearance_count": count,
                        "source": "photoprism-tag",
                        "top_locations": top_locations,
                        "active_months": active_months,
                    },
                )
            )
            facts.append(
                KnowledgeFact(
                    sensor_name=self.name,
                    fact_type="person_appearance_count",
                    subject_key="person:%s" % person.lower().replace(" ", "-"),
                    fact_key="appearance_count",
                    observed_at=now,
                    summary="%s appears in %d indexed photos." % (person, count),
                    payload={"appearance_count": count, "top_locations": top_locations, "active_months": active_months},
                    confidence=0.93,
                )
            )

        for (left, right), count in pair_counter.items():
            relationships.append(
                KnowledgeRelationship(
                    relationship_type="photo_cooccurrence",
                    source_entity_key="person:%s" % left.lower().replace(" ", "-"),
                    target_entity_key="person:%s" % right.lower().replace(" ", "-"),
                    first_seen=now,
                    last_seen=now,
                    strength=float(count),
                    attributes={"cooccurrence_count": count},
                )
            )

        for location, count in location_counter.most_common(10):
            facts.append(
                KnowledgeFact(
                    sensor_name=self.name,
                    fact_type="location_frequency",
                    subject_key="photoprism:library",
                    fact_key="location:%s" % location.lower().replace(" ", "-"),
                    observed_at=now,
                    summary="Location %s appears in %d indexed photos." % (location, count),
                    payload={"location": location, "photo_count": count},
                    confidence=0.88,
                )
            )

        summary = "Indexed %d photos, observed %d tagged people, and found %d recurring locations." % (len(photos), len(people_counter), len(location_counter))
        return SensorRunResult(
            sensor_name=self.name,
            started_at=started_at,
            finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
            status="ok",
            observations=[
                Observation(
                    sensor_name=self.name,
                    observation_type="photoprism.index",
                    subject_key="photoprism:library",
                    title="PhotoPrism indexing pass",
                    summary=summary,
                    observed_at=now,
                    significance=0.62 if photos else 0.2,
                    payload={"photo_count": len(photos), "people_count": len(people_counter), "location_count": len(location_counter)},
                )
            ],
            entities=entities,
            relationships=relationships,
            facts=facts,
            metadata={"indexed_photos": len(photos), "people_count": len(people_counter), "location_count": len(location_counter)},
            commentary_hint="The photo archive suggests recurring human constellations and habits." if photos else None,
        )

    def _fetch_photos(self, base_url, api_key):
        request = Request(
            "%s/api/v1/photos?%s" % (base_url, urlencode({"count": 200})),
            headers={"X-API-Key": api_key},
        )
        with urlopen(request, timeout=30) as response:
            payload = json.loads(response.read().decode("utf-8"))

        normalized = []
        for item in payload:
            people = []
            for label in item.get("Labels", []) or []:
                name = label.get("Name") or label.get("Label")
                if name:
                    people.append(name)
            normalized.append(
                {
                    "uid": item.get("UID") or item.get("ID") or "unknown",
                    "taken_at": item.get("TakenAt") or item.get("CreatedAt") or "",
                    "people": people,
                    "location": item.get("PlaceName") or item.get("Country") or "",
                }
            )
        return normalized