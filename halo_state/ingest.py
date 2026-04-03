from __future__ import annotations

import json
import mimetypes
import os
import urllib.request
from pathlib import Path
from typing import Any

from .config import StateConfig
from .postgres import connect
from .service import PostgresStateService
from .utils import chunk_text, content_hash_bytes

try:
    from pypdf import PdfReader
except ImportError:  # pragma: no cover - optional dependency
    PdfReader = None

try:
    from qdrant_client import QdrantClient
    from qdrant_client.http import models as qdrant_models
except ImportError:  # pragma: no cover - optional dependency
    QdrantClient = None
    qdrant_models = None


def _extract_text(path: Path) -> tuple[str, str, str]:
    suffix = path.suffix.lower()
    if suffix == ".pdf":
        if PdfReader is None:
            raise RuntimeError("pypdf is required to ingest PDF learning material.")
        reader = PdfReader(str(path))
        text = "\n\n".join((page.extract_text() or "") for page in reader.pages).strip()
        return text, "pypdf", "application/pdf"

    text = path.read_text(encoding="utf-8", errors="ignore")
    media_type = mimetypes.guess_type(path.name)[0] or "text/plain"
    return text, "text", media_type


def _embed_texts(config: StateConfig, texts: list[str]) -> list[list[float]]:
    api_key = os.getenv("OPENAI_API_KEY", "").strip()
    if not api_key:
        raise RuntimeError("OPENAI_API_KEY is required for embedding generation.")
    payload = {
        "model": config.embedding_model,
        "input": texts,
    }
    request = urllib.request.Request(
        config.embedding_api_url,
        data=json.dumps(payload).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        },
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=180) as response:
        data = json.loads(response.read().decode("utf-8"))
    return [item["embedding"] for item in data.get("data", [])]


def _qdrant_client(config: StateConfig):
    if not config.qdrant_url or QdrantClient is None:
        return None
    return QdrantClient(url=config.qdrant_url, api_key=config.qdrant_api_key or None)


def ingest_learning_material(config: StateConfig, document_root: Path | None = None, glob_pattern: str | None = None) -> dict[str, int]:
    root = document_root or config.document_root
    pattern = glob_pattern or config.document_glob
    if not root.exists():
        raise RuntimeError(f"Learning material root does not exist: {root}")

    service = PostgresStateService(config)
    qdrant = _qdrant_client(config) if config.enable_vector_index else None
    stats = {
        "documents_seen": 0,
        "documents_indexed": 0,
        "documents_skipped": 0,
        "chunks_written": 0,
        "vectors_written": 0,
    }

    with connect(config.postgres_dsn) as connection:
        service._profile_id(connection)
        for path in sorted(root.glob(pattern)):
            if not path.is_file():
                continue
            stats["documents_seen"] += 1
            try:
                raw_bytes = path.read_bytes()
                text, parser_name, media_type = _extract_text(path)
            except Exception:
                stats["documents_skipped"] += 1
                continue

            normalized_path = str(path).replace("\\", "/")
            content_hash = content_hash_bytes(raw_bytes)
            document_key = content_hash_bytes(normalized_path.lower().encode("utf-8"))
            chunks = chunk_text(text)
            if not chunks:
                stats["documents_skipped"] += 1
                continue

            with connection.cursor() as cursor:
                cursor.execute(
                    f"SELECT id, content_hash FROM {config.postgres_schema}.source_documents WHERE document_key = %s",
                    (document_key,),
                )
                existing = cursor.fetchone()

                if existing and existing["content_hash"] == content_hash:
                    stats["documents_skipped"] += 1
                    continue

                if existing:
                    document_id = existing["id"]
                    cursor.execute(
                        f"""
                        UPDATE {config.postgres_schema}.source_documents
                        SET source_path = %s,
                            content_hash = %s,
                            media_type = %s,
                            parser_name = %s,
                            last_seen_at = NOW(),
                            updated_at = NOW()
                        WHERE id = %s
                        """,
                        (normalized_path, content_hash, media_type, parser_name, document_id),
                    )
                else:
                    cursor.execute(
                        f"""
                        INSERT INTO {config.postgres_schema}.source_documents (
                            document_key, source_path, content_hash, media_type, title, parser_name, metadata_json
                        ) VALUES (%s, %s, %s, %s, %s, %s, %s::jsonb)
                        RETURNING id
                        """,
                        (
                            document_key,
                            normalized_path,
                            content_hash,
                            media_type,
                            path.stem,
                            parser_name,
                            json.dumps({"source_name": path.name}, sort_keys=True),
                        ),
                    )
                    document_id = cursor.fetchone()["id"]

                cursor.execute(
                    f"""
                    INSERT INTO {config.postgres_schema}.document_ingestions (
                        document_id, status, parser_version, metadata_json
                    ) VALUES (%s, %s, %s, %s::jsonb)
                    RETURNING id
                    """,
                    (document_id, "running", parser_name, json.dumps({"path": normalized_path}, sort_keys=True)),
                )
                ingestion_id = cursor.fetchone()["id"]
                cursor.execute(
                    f"DELETE FROM {config.postgres_schema}.source_chunks WHERE document_id = %s",
                    (document_id,),
                )

                point_ids: list[str] = []
                embeddings: list[list[float]] = []
                if qdrant is not None:
                    try:
                        embeddings = _embed_texts(config, [chunk["text_content"] for chunk in chunks])
                        if embeddings and qdrant_models is not None:
                            collection_name = config.qdrant_collection
                            if not qdrant.collection_exists(collection_name):
                                qdrant.create_collection(
                                    collection_name=collection_name,
                                    vectors_config=qdrant_models.VectorParams(
                                        size=len(embeddings[0]),
                                        distance=qdrant_models.Distance.COSINE,
                                    ),
                                )
                            points = []
                            for chunk, embedding in zip(chunks, embeddings):
                                point_id = f"{document_id}:{chunk['chunk_index']}"
                                point_ids.append(point_id)
                                points.append(
                                    qdrant_models.PointStruct(
                                        id=point_id,
                                        vector=embedding,
                                        payload={
                                            "document_id": str(document_id),
                                            "chunk_index": int(chunk["chunk_index"]),
                                            "source_path": normalized_path,
                                            "title": path.stem,
                                        },
                                    )
                                )
                            qdrant.upsert(collection_name=collection_name, points=points)
                    except Exception:
                        point_ids = []
                        embeddings = []

                for chunk in chunks:
                    point_id = ""
                    if point_ids:
                        point_id = point_ids[int(chunk["chunk_index"])]
                    cursor.execute(
                        f"""
                        INSERT INTO {config.postgres_schema}.source_chunks (
                            document_id, chunk_index, heading, text_content, token_count, char_count, qdrant_point_id, metadata_json
                        ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s::jsonb)
                        """,
                        (
                            document_id,
                            int(chunk["chunk_index"]),
                            "",
                            chunk["text_content"],
                            int(chunk["token_count"]),
                            int(chunk["char_count"]),
                            point_id,
                            json.dumps({"source_path": normalized_path}, sort_keys=True),
                        ),
                    )

                cursor.execute(
                    f"""
                    UPDATE {config.postgres_schema}.document_ingestions
                    SET status = %s, chunk_count = %s, finished_at = NOW()
                    WHERE id = %s
                    """,
                    ("completed", len(chunks), ingestion_id),
                )
                connection.commit()
                stats["documents_indexed"] += 1
                stats["chunks_written"] += len(chunks)
                stats["vectors_written"] += len(point_ids)
    return stats
