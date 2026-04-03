import argparse
import json
from pathlib import Path

from .manager import SensorManager


def build_parser():
    parser = argparse.ArgumentParser(prog="halo sensory")
    parser.add_argument("--halo-root", required=True)
    subparsers = parser.add_subparsers(dest="command", required=True)

    status_parser = subparsers.add_parser("status")
    status_parser.set_defaults(command="status")

    scan_parser = subparsers.add_parser("scan")
    scan_parser.add_argument("sensor", nargs="?", default="all")
    scan_parser.set_defaults(command="scan")

    commentary_parser = subparsers.add_parser("commentary")
    commentary_parser.add_argument("sensor", nargs="?", default="all")
    commentary_parser.add_argument("--threshold", type=float, default=0.55)
    commentary_parser.add_argument("--cooldown-seconds", type=int, default=1800)
    commentary_parser.set_defaults(command="commentary")

    mark_parser = subparsers.add_parser("mark-commentary")
    mark_parser.add_argument("--trigger-key", required=True)
    mark_parser.add_argument("--fingerprint", required=True)
    mark_parser.add_argument("--summary", required=True)
    mark_parser.add_argument("--metadata-json", default="{}")
    mark_parser.set_defaults(command="mark-commentary")
    return parser


def main():
    parser = build_parser()
    args = parser.parse_args()
    manager = SensorManager(Path(args.halo_root))

    if args.command == "status":
        print(json.dumps(manager.status(), indent=2, sort_keys=True))
        return 0

    if args.command == "scan":
        print(json.dumps(manager.run(args.sensor), indent=2, sort_keys=True))
        return 0

    if args.command == "commentary":
        print(json.dumps(manager.commentary_candidate(args.sensor, args.threshold, args.cooldown_seconds), indent=2, sort_keys=True))
        return 0

    if args.command == "mark-commentary":
        manager.mark_commentary_emitted(args.trigger_key, args.fingerprint, args.summary, json.loads(args.metadata_json))
        print(json.dumps({"status": "ok", "trigger_key": args.trigger_key}, indent=2, sort_keys=True))
        return 0

    parser.error("Unknown command")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())