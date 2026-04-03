import datetime
import json
import shutil
import subprocess
from pathlib import Path

from ..models import KnowledgeFact, Observation, SensorRunResult
from .base import Sensor


class HostSensor(Sensor):
    name = "host"

    def collect(self) -> SensorRunResult:
        started_at = datetime.datetime.now(datetime.timezone.utc).isoformat()
        now = started_at
        observations = []
        facts = []
        errors = []

        loadavg = self._read_loadavg(errors)
        meminfo = self._read_meminfo(errors)
        uptime = self._read_uptime(errors)
        disk = shutil.disk_usage("/")
        processes = self._top_processes(errors)
        failed_services = self._failed_services(errors)

        summary = (
            f"CPU load {loadavg.get('load1', 0.0):.2f}, memory {meminfo.get('used_percent', 0.0):.1f}% used, "
            f"disk {((disk.used / disk.total) * 100) if disk.total else 0.0:.1f}% used."
        )
        observations.append(
            Observation(
                sensor_name=self.name,
                observation_type="host.health",
                subject_key="host:local",
                title="Local host health snapshot",
                summary=summary,
                observed_at=now,
                significance=0.58,
                payload={
                    "loadavg": loadavg,
                    "memory": meminfo,
                    "uptime_seconds": uptime,
                    "disk": {"used": disk.used, "total": disk.total, "free": disk.free},
                    "top_processes": processes,
                    "failed_services": failed_services,
                },
            )
        )

        facts.extend(
            [
                KnowledgeFact(self.name, "host_metric", "host:local", "load1", now, f"1-minute load is {loadavg.get('load1', 0.0):.2f}", loadavg, 0.92),
                KnowledgeFact(self.name, "host_metric", "host:local", "memory_used_percent", now, f"Memory usage is {meminfo.get('used_percent', 0.0):.1f}%", meminfo, 0.94),
                KnowledgeFact(self.name, "host_metric", "host:local", "disk_used_percent", now, f"Root disk usage is {((disk.used / disk.total) * 100) if disk.total else 0.0:.1f}%", {"used_percent": ((disk.used / disk.total) * 100) if disk.total else 0.0}, 0.95),
                KnowledgeFact(self.name, "host_metric", "host:local", "uptime_seconds", now, f"System uptime is {uptime:.0f} seconds", {"uptime_seconds": uptime}, 0.99),
            ]
        )

        if failed_services:
            observations.append(
                Observation(
                    sensor_name=self.name,
                    observation_type="host.services.failed",
                    subject_key="host:local",
                    title="Failed services detected",
                    summary=f"{len(failed_services)} systemd services are currently failed.",
                    observed_at=now,
                    significance=0.82,
                    payload={"failed_services": failed_services},
                )
            )

        commentary_hint = None
        if failed_services:
            commentary_hint = "The host body is showing service failures that may warrant attention."

        return SensorRunResult(
            sensor_name=self.name,
            started_at=started_at,
            finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
            status="ok" if not errors else "degraded",
            observations=observations,
            facts=facts,
            metadata={"process_count": len(processes), "failed_service_count": len(failed_services)},
            errors=errors,
            commentary_hint=commentary_hint,
        )

    def _read_loadavg(self, errors):
        try:
            values = Path("/proc/loadavg").read_text(encoding="utf-8").split()
            return {"load1": float(values[0]), "load5": float(values[1]), "load15": float(values[2])}
        except Exception as exc:
            errors.append("loadavg: %s" % exc)
            return {"load1": 0.0, "load5": 0.0, "load15": 0.0}

    def _read_meminfo(self, errors):
        try:
            parsed = {}
            for line in Path("/proc/meminfo").read_text(encoding="utf-8").splitlines():
                key, value = line.split(":", 1)
                parsed[key.strip()] = float(value.strip().split()[0])
            total = parsed.get("MemTotal", 0.0)
            available = parsed.get("MemAvailable", 0.0)
            used = max(total - available, 0.0)
            return {
                "total_kb": total,
                "available_kb": available,
                "used_kb": used,
                "used_percent": (used / total * 100.0) if total else 0.0,
            }
        except Exception as exc:
            errors.append("meminfo: %s" % exc)
            return {"total_kb": 0.0, "available_kb": 0.0, "used_kb": 0.0, "used_percent": 0.0}

    def _read_uptime(self, errors):
        try:
            return float(Path("/proc/uptime").read_text(encoding="utf-8").split()[0])
        except Exception as exc:
            errors.append("uptime: %s" % exc)
            return 0.0

    def _top_processes(self, errors):
        try:
            output = subprocess.check_output(
                ["ps", "-eo", "pid=,comm=,%cpu=,%mem=", "--sort=-%cpu"],
                text=True,
                stderr=subprocess.DEVNULL,
            )
            processes = []
            for line in output.splitlines()[:5]:
                parts = line.split(None, 3)
                if len(parts) != 4:
                    continue
                processes.append(
                    {"pid": int(parts[0]), "command": parts[1], "cpu_percent": float(parts[2]), "memory_percent": float(parts[3])}
                )
            return processes
        except Exception as exc:
            errors.append("processes: %s" % exc)
            return []

    def _failed_services(self, errors):
        try:
            output = subprocess.check_output(
                ["systemctl", "--failed", "--no-legend", "--plain"],
                text=True,
                stderr=subprocess.DEVNULL,
            )
            return [line.strip() for line in output.splitlines() if line.strip()]
        except Exception as exc:
            errors.append("failed_services: %s" % exc)
            return []