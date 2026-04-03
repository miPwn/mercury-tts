from abc import ABC, abstractmethod
from pathlib import Path
from typing import Dict

from ..models import SensorRunResult


class Sensor(ABC):
    name = "sensor"

    def __init__(self, halo_root: Path) -> None:
        self.halo_root = halo_root

    @abstractmethod
    def collect(self) -> SensorRunResult:
        raise NotImplementedError

    def env(self, key: str, default: str = "") -> str:
        import os

        return os.environ.get(key, default)

    def configured(self) -> Dict[str, object]:
        return {"enabled": True}