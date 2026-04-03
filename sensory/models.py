from dataclasses import dataclass, field
from typing import Dict, List, Optional


@dataclass
class Observation:
    sensor_name: str
    observation_type: str
    subject_key: str
    title: str
    summary: str
    observed_at: str
    significance: float
    payload: Dict[str, object] = field(default_factory=dict)


@dataclass
class KnowledgeEntity:
    entity_type: str
    entity_key: str
    display_name: str
    first_seen: str
    last_seen: str
    attributes: Dict[str, object] = field(default_factory=dict)


@dataclass
class KnowledgeRelationship:
    relationship_type: str
    source_entity_key: str
    target_entity_key: str
    first_seen: str
    last_seen: str
    strength: float
    attributes: Dict[str, object] = field(default_factory=dict)


@dataclass
class KnowledgeFact:
    sensor_name: str
    fact_type: str
    subject_key: str
    fact_key: str
    observed_at: str
    summary: str
    payload: Dict[str, object] = field(default_factory=dict)
    confidence: float = 0.5


@dataclass
class SensorRunResult:
    sensor_name: str
    started_at: str
    finished_at: str
    status: str
    observations: List[Observation] = field(default_factory=list)
    entities: List[KnowledgeEntity] = field(default_factory=list)
    relationships: List[KnowledgeRelationship] = field(default_factory=list)
    facts: List[KnowledgeFact] = field(default_factory=list)
    metadata: Dict[str, object] = field(default_factory=dict)
    errors: List[str] = field(default_factory=list)
    commentary_hint: Optional[str] = None