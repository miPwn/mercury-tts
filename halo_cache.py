from __future__ import annotations

import json
import pathlib
import re
import tempfile
import unicodedata
import uuid
import wave
from typing import Iterable


def slugify_name(value: str, fallback: str = "assembled") -> str:
    normalized = unicodedata.normalize("NFKD", value or "")
    ascii_text = normalized.encode("ascii", "ignore").decode("ascii")
    words = re.findall(r"[A-Za-z0-9]+", ascii_text.lower())
    slug = "-".join(words[:16]).strip("-")
    return slug or fallback


def new_guid_wav_path(directory: pathlib.Path) -> str:
    directory.mkdir(parents=True, exist_ok=True)
    return str(directory / f"{uuid.uuid4()}.wav")


def write_cache_index_file(index_file: pathlib.Path, payload: dict) -> None:
    index_file.parent.mkdir(parents=True, exist_ok=True)
    temp_file = index_file.with_suffix(index_file.suffix + ".tmp")
    temp_file.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
    temp_file.replace(index_file)


def read_cache_index_file(index_file: pathlib.Path) -> dict:
    if not index_file.is_file() or index_file.stat().st_size == 0:
        return {}
    try:
        return json.loads(index_file.read_text(encoding="utf-8"))
    except Exception:
        return {}


def _manifest_audio_paths(manifest_path: pathlib.Path) -> list[pathlib.Path]:
    audio_paths: list[pathlib.Path] = []
    for raw_line in manifest_path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line:
            continue
        audio_path = line.split("\t", 1)[0].strip()
        if audio_path:
            audio_paths.append(pathlib.Path(audio_path))
    return audio_paths


def assemble_wav_playlist(manifest_path: pathlib.Path, output_path: pathlib.Path) -> None:
    audio_paths = _manifest_audio_paths(manifest_path)
    if not audio_paths:
        raise ValueError("playlist is empty")

    with wave.open(str(audio_paths[0]), "rb") as first_source:
        params = first_source.getparams()
        frames = [first_source.readframes(first_source.getnframes())]
        expected_signature = (
            params.nchannels,
            params.sampwidth,
            params.framerate,
            params.comptype,
            params.compname,
        )

    for path in audio_paths[1:]:
        with wave.open(str(path), "rb") as source:
            signature = (
                source.getnchannels(),
                source.getsampwidth(),
                source.getframerate(),
                source.getcomptype(),
                source.getcompname(),
            )
            if signature != expected_signature:
                raise ValueError(f"wav parameters do not match for {path}")
            frames.append(source.readframes(source.getnframes()))

    output_path.parent.mkdir(parents=True, exist_ok=True)
    with wave.open(str(output_path), "wb") as target:
        target.setparams(params)
        for frame_blob in frames:
            target.writeframes(frame_blob)
