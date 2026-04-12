from __future__ import annotations

import datetime as _dt
import hashlib
import re
from pathlib import Path
from typing import Iterable

_STOPWORDS = {
    "a",
    "an",
    "and",
    "are",
    "as",
    "at",
    "be",
    "by",
    "for",
    "from",
    "how",
    "i",
    "in",
    "is",
    "it",
    "me",
    "my",
    "of",
    "on",
    "or",
    "that",
    "the",
    "their",
    "them",
    "they",
    "this",
    "to",
    "was",
    "we",
    "what",
    "when",
    "where",
    "who",
    "why",
    "with",
    "you",
    "your",
}


def now_iso() -> str:
    return _dt.datetime.now(_dt.timezone.utc).isoformat()


def collapse_ws(value: str) -> str:
    return " ".join((value or "").split()).strip()


def summarize_text(value: str, max_chars: int = 260) -> str:
    return collapse_ws(value)[:max_chars]


def tokenize(value: str) -> set[str]:
    return {
        token
        for token in re.findall(r"[a-z0-9]+", (value or "").lower())
        if token not in _STOPWORDS and (len(token) > 2 or token.isdigit())
    }


def build_memory_payload(
    rows: Iterable[dict[str, str]],
    kind: str,
    topic: str,
    trigger_source: str,
    recent_limit: int,
    relevant_limit: int,
    summary: str,
) -> dict[str, object]:
    payload: dict[str, object] = {
        "summary": summary,
        "recent": [],
        "relevant": [],
        "query": {
            "kind": kind,
            "topic": topic,
            "trigger_source": trigger_source,
        },
    }
    entries = list(rows)
    payload["recent"] = entries[:recent_limit]
    recent_keys = {
        (entry["created_at"], entry["summary_snippet"]) for entry in payload["recent"]
    }

    query_tokens = tokenize(topic)
    query_tokens.update(tokenize(kind))
    query_tokens.update(tokenize(trigger_source))

    scored: list[tuple[int, int, dict[str, str]]] = []
    for index, entry in enumerate(entries[recent_limit:], start=recent_limit):
        text_tokens = tokenize(entry.get("topic", ""))
        text_tokens.update(tokenize(entry.get("summary_snippet", "")))
        text_tokens.update(tokenize(entry.get("text_excerpt", "")))
        overlap = len(query_tokens & text_tokens)
        recency_bonus = max(0, 12 - index)
        score = overlap * 10 + recency_bonus
        scored.append((score, index, entry))

    scored.sort(key=lambda item: (-item[0], item[1]))
    payload["relevant"] = [
        entry
        for score, _, entry in scored
        if score > 0
        and (entry["created_at"], entry["summary_snippet"]) not in recent_keys
    ][:relevant_limit]
    return payload


def build_summary_text(rows: Iterable[dict[str, str]], max_chars: int) -> str:
    lines = [
        "HAL aware continuity digest",
        "Use this as compressed memory, not as an exhaustive transcript.",
        "",
    ]
    for row in rows:
        topic_suffix = f" | topic: {row['topic']}" if row.get("topic") else ""
        lines.append(
            f"- {row['created_at']} | {row['kind']} | trigger: {row['trigger_source']}{topic_suffix}"
        )
        lines.append(f"  {row['summary_snippet']}")
    text = "\n".join(lines).strip() + "\n"
    if len(text) > max_chars:
        text = text[:max_chars].rstrip() + "\n"
    return text


def mask_dsn(dsn: str) -> str:
    if not dsn or "@" not in dsn or "://" not in dsn:
        return dsn
    prefix, rest = dsn.split("://", 1)
    creds, host = rest.split("@", 1)
    if ":" in creds:
        user, _password = creds.split(":", 1)
        return f"{prefix}://{user}:***@{host}"
    return dsn


def content_hash_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def chunk_text(text: str, max_chars: int = 1200) -> list[dict[str, object]]:
    text = text.replace("\r\n", "\n").replace("\r", "\n").strip()
    if not text:
        return []

    paragraphs = [part.strip() for part in re.split(r"\n\s*\n+", text) if part.strip()]
    chunks: list[str] = []
    buffer = ""
    for paragraph in paragraphs:
        candidate = f"{buffer}\n\n{paragraph}".strip() if buffer else paragraph
        if buffer and len(candidate) > max_chars:
            chunks.append(buffer)
            buffer = ""
        if len(paragraph) > max_chars:
            if buffer:
                chunks.append(buffer)
                buffer = ""
            sentences = [
                part.strip()
                for part in re.split(r"(?<=[.!?])\s+", paragraph)
                if part.strip()
            ]
            sentence_buffer = ""
            for sentence in sentences:
                sentence_candidate = (
                    f"{sentence_buffer} {sentence}".strip()
                    if sentence_buffer
                    else sentence
                )
                if sentence_buffer and len(sentence_candidate) > max_chars:
                    chunks.append(sentence_buffer)
                    sentence_buffer = sentence
                    continue
                if len(sentence) > max_chars:
                    words = sentence.split()
                    word_buffer = []
                    current_len = 0
                    for word in words:
                        candidate_len = (
                            current_len + (1 if word_buffer else 0) + len(word)
                        )
                        if word_buffer and candidate_len > max_chars:
                            chunks.append(" ".join(word_buffer))
                            word_buffer = [word]
                            current_len = len(word)
                            continue
                        word_buffer.append(word)
                        current_len = candidate_len
                    if word_buffer:
                        chunks.append(" ".join(word_buffer))
                    sentence_buffer = ""
                    continue
                sentence_buffer = sentence_candidate
            if sentence_buffer:
                chunks.append(sentence_buffer)
            continue
        buffer = candidate
    if buffer:
        chunks.append(buffer)

    return [
        {
            "chunk_index": index,
            "text_content": chunk,
            "char_count": len(chunk),
            "token_count": len(chunk.split()),
        }
        for index, chunk in enumerate(chunks)
    ]


def ensure_parent(path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
