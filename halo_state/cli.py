from __future__ import annotations

import argparse
import base64
import json
import os
import sys
from pathlib import Path

from .config import StateConfig
from .ingest import ingest_learning_material
from .service import PostgresStateService


def _service() -> PostgresStateService:
    return PostgresStateService(StateConfig.from_env())


def cmd_aware_status(args: argparse.Namespace) -> int:
    service = _service()
    text = service.status(
        state_file=Path(args.state_file),
        summary_file=Path(args.summary_file),
        trigger_file=Path(args.trigger_file),
        output_dir=Path(args.output_dir),
        legacy_memory_db=Path(args.memory_db),
    )
    print(text)
    return 0


def cmd_select_context(args: argparse.Namespace) -> int:
    service = _service()
    payload = service.select_memory_context(
        kind=args.kind,
        topic=args.topic,
        trigger_source=args.trigger_source,
        recent_limit=args.recent_limit,
        relevant_limit=args.relevant_limit,
    )
    print(json.dumps(payload))
    return 0


def cmd_refresh_summary(args: argparse.Namespace) -> int:
    service = _service()
    service.refresh_summary(Path(args.summary_file), args.entry_limit, args.max_chars)
    return 0


def cmd_record_aware_output(args: argparse.Namespace) -> int:
    service = _service()
    text = os.environ.get("HALO_STATE_OUTPUT_TEXT", "")
    if not text and args.text_base64:
        text = base64.b64decode(args.text_base64.encode("ascii")).decode("utf-8")
    if not text and args.text_file:
        text = Path(args.text_file).read_text(encoding="utf-8")
    if not text:
        raise RuntimeError("HALO_STATE_OUTPUT_TEXT is not set and --text-file was not provided.")
    service.record_aware_output(
        kind=args.kind,
        topic=args.topic,
        trigger_source=args.trigger_source,
        trigger_id=args.trigger_id,
        text=text,
        artifact_path=args.artifact_path,
    )
    return 0


def cmd_evaluate_triggers(args: argparse.Namespace) -> int:
    service = _service()
    payload = service.evaluate_triggers(Path(args.trigger_file))
    if payload:
        print(json.dumps(payload))
    return 0


def cmd_migrate_legacy_state(args: argparse.Namespace) -> int:
    service = _service()
    stats = service.migrate_legacy_state(
        aware_db=Path(args.aware_db) if args.aware_db else None,
        sensory_db=Path(args.sensory_db) if args.sensory_db else None,
        commentary_db=Path(args.commentary_db) if args.commentary_db else None,
    )
    print(json.dumps(stats, indent=2, sort_keys=True))
    return 0


def cmd_ingest_learning_material(args: argparse.Namespace) -> int:
    config = StateConfig.from_env()
    stats = ingest_learning_material(
        config,
        document_root=Path(args.document_root) if args.document_root else None,
        glob_pattern=args.glob_pattern,
    )
    print(json.dumps(stats, indent=2, sort_keys=True))
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="HAL state management CLI")
    subparsers = parser.add_subparsers(dest="command", required=True)

    aware_status = subparsers.add_parser("aware-status")
    aware_status.add_argument("--state-file", required=True)
    aware_status.add_argument("--memory-db", required=True)
    aware_status.add_argument("--summary-file", required=True)
    aware_status.add_argument("--trigger-file", required=True)
    aware_status.add_argument("--output-dir", required=True)
    aware_status.set_defaults(func=cmd_aware_status)

    select_context = subparsers.add_parser("select-context")
    select_context.add_argument("--kind", required=True)
    select_context.add_argument("--topic", default="")
    select_context.add_argument("--trigger-source", required=True)
    select_context.add_argument("--recent-limit", type=int, required=True)
    select_context.add_argument("--relevant-limit", type=int, required=True)
    select_context.set_defaults(func=cmd_select_context)

    refresh_summary = subparsers.add_parser("refresh-summary")
    refresh_summary.add_argument("--summary-file", required=True)
    refresh_summary.add_argument("--entry-limit", type=int, required=True)
    refresh_summary.add_argument("--max-chars", type=int, required=True)
    refresh_summary.set_defaults(func=cmd_refresh_summary)

    record_output = subparsers.add_parser("record-aware-output")
    record_output.add_argument("--kind", required=True)
    record_output.add_argument("--topic", default="")
    record_output.add_argument("--trigger-source", required=True)
    record_output.add_argument("--trigger-id", default="")
    record_output.add_argument("--artifact-path", required=True)
    record_output.add_argument("--text-base64")
    record_output.add_argument("--text-file")
    record_output.set_defaults(func=cmd_record_aware_output)

    evaluate_triggers = subparsers.add_parser("evaluate-triggers")
    evaluate_triggers.add_argument("--trigger-file", required=True)
    evaluate_triggers.set_defaults(func=cmd_evaluate_triggers)

    migrate = subparsers.add_parser("migrate-legacy-state")
    migrate.add_argument("--aware-db")
    migrate.add_argument("--sensory-db")
    migrate.add_argument("--commentary-db")
    migrate.set_defaults(func=cmd_migrate_legacy_state)

    ingest = subparsers.add_parser("ingest-learning-material")
    ingest.add_argument("--document-root")
    ingest.add_argument("--glob-pattern", default=None)
    ingest.set_defaults(func=cmd_ingest_learning_material)

    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
