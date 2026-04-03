import datetime
import os
import re
import socket
import subprocess

from ..models import KnowledgeEntity, KnowledgeFact, Observation, SensorRunResult
from .base import Sensor


class NetworkSensor(Sensor):
    name = "network"

    def collect(self) -> SensorRunResult:
        started_at = datetime.datetime.now(datetime.timezone.utc).isoformat()
        now = started_at
        devices = self._discover_devices()
        observations = []
        entities = []
        facts = []

        if devices:
            observations.append(
                Observation(
                    sensor_name=self.name,
                    observation_type="network.devices.present",
                    subject_key="network:lan",
                    title="LAN device visibility snapshot",
                    summary=f"Observed {len(devices)} devices on the local network.",
                    observed_at=now,
                    significance=0.45,
                    payload={"devices": devices},
                )
            )

        for device in devices:
            entity_key = "device:%s" % (device.get("mac") or device.get("ip") or device.get("hostname") or "unknown")
            display_name = device.get("hostname") or device.get("ip") or entity_key
            entities.append(
                KnowledgeEntity(
                    entity_type="device",
                    entity_key=entity_key,
                    display_name=display_name,
                    first_seen=now,
                    last_seen=now,
                    attributes=device,
                )
            )
            facts.append(
                KnowledgeFact(
                    sensor_name=self.name,
                    fact_type="device_presence",
                    subject_key=entity_key,
                    fact_key="present_on_lan",
                    observed_at=now,
                    summary="Device %s is visible on the LAN." % display_name,
                    payload=device,
                    confidence=0.74,
                )
            )

        return SensorRunResult(
            sensor_name=self.name,
            started_at=started_at,
            finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
            status="ok",
            observations=observations,
            entities=entities,
            facts=facts,
            metadata={"device_count": len(devices)},
        )

    def _discover_devices(self):
        devices = []
        candidates = [self._parse_ip_neigh(), self._parse_arp()]
        seen = set()
        for source in candidates:
            for item in source:
                key = (item.get("mac"), item.get("ip"))
                if key in seen:
                    continue
                seen.add(key)
                devices.append(item)
        return devices

    def _parse_ip_neigh(self):
        try:
            output = subprocess.check_output(["ip", "neigh"], text=True, stderr=subprocess.DEVNULL)
        except Exception:
            return []
        devices = []
        for line in output.splitlines():
            match = re.search(r"^(?P<ip>\S+).*lladdr\s+(?P<mac>[0-9a-f:]{17})\s+(?P<state>\S+)$", line)
            if not match:
                continue
            ip_address = match.group("ip")
            devices.append(
                {
                    "ip": ip_address,
                    "mac": match.group("mac"),
                    "state": match.group("state"),
                    "hostname": self._safe_hostname(ip_address),
                    "source": "ip-neigh",
                }
            )
        return devices

    def _parse_arp(self):
        try:
            output = subprocess.check_output(["arp", "-an"], text=True, stderr=subprocess.DEVNULL)
        except Exception:
            return []
        devices = []
        for line in output.splitlines():
            match = re.search(r"\((?P<ip>[^)]+)\)\s+at\s+(?P<mac>[0-9a-f:]{17}|<incomplete>)", line)
            if not match or match.group("mac") == "<incomplete>":
                continue
            ip_address = match.group("ip")
            devices.append(
                {
                    "ip": ip_address,
                    "mac": match.group("mac"),
                    "state": "reachable",
                    "hostname": self._safe_hostname(ip_address),
                    "source": "arp",
                }
            )
        return devices

    def _safe_hostname(self, ip_address):
        try:
            return socket.gethostbyaddr(ip_address)[0]
        except Exception:
            return ""