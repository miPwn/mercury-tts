from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path


def _bool_env(name: str, default: bool) -> bool:
    value = os.getenv(name)
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}


def _int_env(name: str, default: int) -> int:
    value = os.getenv(name)
    if value is None:
        return default
    return int(value.strip())


@dataclass(frozen=True)
class StateConfig:
    postgres_dsn: str = ""
    postgres_schema: str = "halo"
    profile_key: str = "hal-9000"
    qdrant_url: str = ""
    qdrant_api_key: str = ""
    qdrant_collection: str = "halo-memory"
    document_root: Path = Path("Z:/hal-system-monitor/learning_matrial")
    document_glob: str = "**/*"
    embedding_model: str = "text-embedding-3-small"
    embedding_api_url: str = "https://api.openai.com/v1/embeddings"
    retrieval_top_k: int = 8
    memory_top_k: int = 6
    canon_top_k: int = 4
    enable_vector_index: bool = True
    enable_learning_ingest: bool = True

    @classmethod
    def from_env(cls) -> "StateConfig":
        return cls(
            postgres_dsn=os.getenv("HALO_STATE_POSTGRES_DSN", "").strip(),
            postgres_schema=os.getenv("HALO_STATE_POSTGRES_SCHEMA", "halo").strip() or "halo",
            profile_key=os.getenv("HALO_STATE_PROFILE_KEY", "hal-9000").strip() or "hal-9000",
            qdrant_url=os.getenv("HALO_STATE_QDRANT_URL", "").strip(),
            qdrant_api_key=os.getenv("HALO_STATE_QDRANT_API_KEY", "").strip(),
            qdrant_collection=os.getenv("HALO_STATE_QDRANT_COLLECTION", "halo-memory").strip() or "halo-memory",
            document_root=Path(os.getenv("HALO_STATE_DOCUMENT_ROOT", "Z:/hal-system-monitor/learning_matrial")),
            document_glob=os.getenv("HALO_STATE_DOCUMENT_GLOB", "**/*").strip() or "**/*",
            embedding_model=os.getenv("HALO_STATE_EMBEDDING_MODEL", "text-embedding-3-small").strip() or "text-embedding-3-small",
            embedding_api_url=os.getenv("HALO_STATE_EMBEDDING_API_URL", "https://api.openai.com/v1/embeddings").strip() or "https://api.openai.com/v1/embeddings",
            retrieval_top_k=_int_env("HALO_STATE_RETRIEVAL_TOP_K", 8),
            memory_top_k=_int_env("HALO_STATE_MEMORY_TOP_K", 6),
            canon_top_k=_int_env("HALO_STATE_CANON_TOP_K", 4),
            enable_vector_index=_bool_env("HALO_STATE_ENABLE_VECTOR_INDEX", True),
            enable_learning_ingest=_bool_env("HALO_STATE_ENABLE_LEARNING_INGEST", True),
        )
