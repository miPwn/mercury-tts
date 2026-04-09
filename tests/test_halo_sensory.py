import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from sensory.knowledge_store import KnowledgeStore
from sensory.manager import SensorManager
from sensory.models import KnowledgeEntity, Observation, SensorRunResult
from sensory.sensors.network import NetworkSensor
from sensory.sensors.photoprism import PhotoPrismSensor


@unittest.skipUnless(os.getenv("HALO_STATE_POSTGRES_DSN"), "HALO_STATE_POSTGRES_DSN is required for KnowledgeStore tests.")
class KnowledgeStoreTests(unittest.TestCase):
    def test_commentary_cooldown(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            store = KnowledgeStore()
            now = "2026-04-03T10:00:00+00:00"
            self.assertTrue(store.commentary_allowed("sensory:host", "abc", 1800, now))
            store.record_commentary_emission("sensory:host", "abc", "test", now, {"sensor": "host"})
            self.assertFalse(store.commentary_allowed("sensory:host", "abc", 1800, "2026-04-03T10:10:00+00:00"))
            self.assertFalse(store.commentary_allowed("sensory:host", "def", 1800, "2026-04-03T10:10:00+00:00"))
            self.assertTrue(store.commentary_allowed("sensory:host", "def", 1800, "2026-04-03T10:40:01+00:00"))


@unittest.skipUnless(os.getenv("HALO_STATE_POSTGRES_DSN"), "HALO_STATE_POSTGRES_DSN is required for SensorManager tests.")
class ManagerTests(unittest.TestCase):
    def test_commentary_candidate_and_status(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            manager = SensorManager(root)
            manager.sensors = {
                "host": FakeSensor(
                    SensorRunResult(
                        sensor_name="host",
                        started_at="2026-04-03T10:00:00+00:00",
                        finished_at="2026-04-03T10:00:01+00:00",
                        status="ok",
                        observations=[
                            Observation(
                                sensor_name="host",
                                observation_type="host.health",
                                subject_key="host:local",
                                title="Health",
                                summary="CPU load has climbed and deserves attention.",
                                observed_at="2026-04-03T10:00:01+00:00",
                                significance=0.9,
                            )
                        ],
                        entities=[
                            KnowledgeEntity(
                                entity_type="device",
                                entity_key="device:test",
                                display_name="test device",
                                first_seen="2026-04-03T10:00:00+00:00",
                                last_seen="2026-04-03T10:00:01+00:00",
                            )
                        ],
                    )
                )
            }

            candidate = manager.commentary_candidate("host", threshold=0.5, cooldown_seconds=1800)
            self.assertTrue(candidate["should_generate"])
            self.assertIn("CPU load has climbed", candidate["candidate"]["summary"])

            manager.mark_commentary_emitted(candidate["trigger_key"], candidate["candidate"]["fingerprint"], candidate["candidate"]["summary"], {"sensor": "host"})
            second = manager.commentary_candidate("host", threshold=0.5, cooldown_seconds=1800)
            self.assertFalse(second["should_generate"])
            self.assertEqual(second["reason"], "cooldown")

            status = manager.status()
            self.assertEqual(status["observation_count"], 2)
            self.assertEqual(status["entity_count"], 1)


class PhotoPrismSensorTests(unittest.TestCase):
    def test_extracts_people_relationships_and_location_facts(self):
        sensor = PhotoPrismSensor(Path("/tmp/halo"))
        with mock.patch.dict("os.environ", {"HAL_PHOTOPRISM_URL": "http://example", "HAL_PHOTOPRISM_API_KEY": "token"}):
            with mock.patch.object(sensor, "_fetch_photos", return_value=[
                {"uid": "1", "taken_at": "2026-01-01T10:00:00Z", "people": ["Alice", "Bob"], "location": "Kitchen"},
                {"uid": "2", "taken_at": "2026-01-02T10:00:00Z", "people": ["Alice"], "location": "Kitchen"},
                {"uid": "3", "taken_at": "2026-02-02T10:00:00Z", "people": ["Bob"], "location": "Garden"},
            ]):
                result = sensor.collect()

        self.assertEqual(result.status, "ok")
        self.assertEqual(len(result.entities), 2)
        self.assertEqual(len(result.relationships), 1)
        self.assertTrue(any(fact.fact_type == "location_frequency" for fact in result.facts))
        self.assertTrue(any(entity.display_name == "Alice" for entity in result.entities))


class NetworkSensorTests(unittest.TestCase):
    def test_discovers_devices_from_ip_neigh_and_arp(self):
        sensor = NetworkSensor(Path("/tmp/halo"))
        ip_neigh = "192.168.1.10 dev eth0 lladdr aa:bb:cc:dd:ee:ff REACHABLE\n"
        arp = "? (192.168.1.11) at 11:22:33:44:55:66 [ether] on eth0\n"

        def fake_check_output(args, text=True, stderr=None):
            if args[:2] == ["ip", "neigh"]:
                return ip_neigh
            if args[:2] == ["arp", "-an"]:
                return arp
            raise AssertionError(args)

        with mock.patch("subprocess.check_output", side_effect=fake_check_output):
            with mock.patch("socket.gethostbyaddr", side_effect=[("router.local", [], []), ("camera.local", [], [])]):
                result = sensor.collect()

        self.assertEqual(result.metadata["device_count"], 2)
        self.assertEqual(len(result.entities), 2)
        self.assertEqual(result.status, "ok")


class FakeSensor:
    def __init__(self, result):
        self._result = result

    def collect(self):
        return self._result


if __name__ == "__main__":
    unittest.main()
