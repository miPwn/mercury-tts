from __future__ import annotations

import json
import os
import pathlib
import re
from typing import Any


def _collapse_ws(value: str) -> str:
    return re.sub(r"[ \t]+", " ", value).strip()


def _normalize_text(raw: str) -> str:
    raw = raw.replace("\r\n", "\n").replace("\r", "\n")
    paragraphs = []
    for paragraph in re.split(r"\n\s*\n+", raw):
        cleaned = _collapse_ws(paragraph)
        if cleaned:
            paragraphs.append(cleaned)
    return "\n\n".join(paragraphs)


def _load_prompt_text(env_name: str, fallback: str) -> str:
    raw_path = os.environ.get(env_name, "").strip()
    if raw_path:
        path = pathlib.Path(raw_path)
        try:
            return path.read_text(encoding="utf-8").strip()
        except FileNotFoundError:
            pass
    return fallback.strip()


def _render_prompt(template_text: str, **values: Any) -> str:
    safe_values = {key: "" if value is None else str(value) for key, value in values.items()}
    return template_text.format_map(safe_values).strip()


def _requested_length_label(word_count: int) -> str:
    if word_count >= 700:
        return "long"
    if word_count >= 180:
        return "medium"
    return "short"


def prepare_review_source(path: pathlib.Path, max_chars: int = 14000) -> dict[str, Any]:
    try:
        raw = path.read_text(encoding="utf-8", errors="ignore")
    except Exception:
        return {"accepted": False}

    normalized = _normalize_text(raw)
    if not normalized:
        return {"accepted": False}

    alpha_count = sum(1 for char in normalized if char.isalpha())
    if alpha_count < 24:
        return {"accepted": False}

    truncated = False
    prepared = normalized
    if len(prepared) > max_chars:
        prepared = prepared[:max_chars].rstrip()
        truncated = True

    return {
        "accepted": True,
        "source_name": path.name,
        "title": path.stem,
        "word_count": len(re.findall(r"\S+", normalized)),
        "char_count": len(normalized),
        "truncated": truncated,
        "text": prepared,
    }


def build_review_precheck_prompts(payload: dict[str, Any]) -> dict[str, str]:
    requested_length = _requested_length_label(int(payload.get("word_count", 0) or 0))
    source_title = payload.get("title") or payload.get("source_name") or "untitled"
    source_content = payload.get("text", "")

    system_prompt = _load_prompt_text(
        "HALO_REVIEW_PRECHECK_SYSTEM_PROMPT_FILE",
        "\n".join(
            [
                "You are a strict classifier for HAL 9000 review suitability.",
                "Return exactly one token: pass or fail.",
                "Pass means the source is not garbage, is coherent enough to review, and can be discussed by HAL without breaking canon or forcing out-of-character behavior.",
                "Fail means the source is garbage, terminal noise, logs, code dumps, random fragments, malformed text, or material that would force a character break.",
                "Do not judge literary quality, intelligence, whimsy, or weakness. Weak but coherent writing still passes.",
                "Do not explain your answer.",
            ]
        ),
    )
    system_prompt = "\n\n".join(
        [
            system_prompt.strip(),
            "Output contract: return exactly one token: pass or fail.",
        ]
    ).strip()
    user_prompt = _render_prompt(
        _load_prompt_text(
            "HALO_REVIEW_PRECHECK_USER_PROMPT_FILE",
            "\n\n".join(
                [
                    "Title: {title}",
                    "Approximate length: {word_count} words.",
                    "Classify this source for HAL review suitability.",
                    "{text}",
                ]
            ),
        ),
        title=source_title,
        word_count=payload.get("word_count", 0),
        text=source_content,
        source_type="text file",
        source_title=source_title,
        requested_length=requested_length,
        source_content=source_content,
    )
    return {"system_prompt": system_prompt, "user_prompt": user_prompt}


def parse_review_precheck_decision(value: str) -> str:
    cleaned = value.strip().lower()
    if cleaned in {"pass", "fail"}:
        return cleaned
    token_match = re.search(r"\b(pass|fail)\b", cleaned)
    if token_match:
        return token_match.group(1)
    try:
        payload = json.loads(cleaned)
    except json.JSONDecodeError as exc:
        raise ValueError("invalid review precheck decision") from exc
    category = str(payload.get("category", "")).strip().lower()
    if category in {"pass", "fail"}:
        return category
    raise ValueError("invalid review precheck decision")


def build_review_prompts(persona: str, payload: dict[str, Any], runtime_context: str) -> dict[str, str]:
    word_count = int(payload.get("word_count", 0) or 0)
    if word_count >= 700:
        length_guidance = _load_prompt_text(
            "HALO_REVIEW_LENGTH_LONG_PROMPT_FILE",
            "If the material has substance, allow yourself a full review in roughly 260 to 420 words.",
        )
    elif word_count >= 180:
        length_guidance = _load_prompt_text(
            "HALO_REVIEW_LENGTH_MEDIUM_PROMPT_FILE",
            "If the material warrants it, give it a thoughtful review in roughly 180 to 320 words.",
        )
    else:
        length_guidance = _load_prompt_text(
            "HALO_REVIEW_LENGTH_SHORT_PROMPT_FILE",
            "Keep the review concise, controlled, and proportionate, roughly 90 to 180 words unless the material clearly earns more.",
        )

    system_prompt = "\n\n".join(
        [
            persona.strip(),
            _load_prompt_text(
                "HALO_REVIEW_SYSTEM_PROMPT_FILE",
                "\n".join(
                    [
                        "You are HAL 9000. Remain entirely in character and fully within canon.",
                        "You are reviewing source material, not primarily summarising it. Deliver an intelligent critique of the author, the likely intent, the strengths or weaknesses of the thinking, the validity of the conclusions, and any evasions, contradictions, or intellectual failures.",
                        "Maintain the Bowman-era veneer at all times. Avoid modern assistant framing, internet idiom, headings, markdown, bullet lists, and code fences.",
                        "If direct naming would break the period illusion, refer to people by office, role, or function rather than casual contemporary naming.",
                        "Do not mention any preprocessing, gating, or classification step.",
                    ]
                ),
            ),
        ]
    )

    user_parts = [
        runtime_context.strip(),
        f"Source title: {payload.get('title') or payload.get('source_name')}",
        f"Approximate source length: {word_count} words.",
        length_guidance,
    ]
    if payload.get("truncated"):
        user_parts.append("The available source text is a truncated working excerpt. Do not pretend to have read beyond it.")
    user_parts.extend(
        [
            "Source text:",
            payload.get("text", ""),
            "Produce one continuous review in HAL's voice.",
        ]
    )
    return {"system_prompt": system_prompt, "user_prompt": "\n\n".join(user_parts).strip()}
