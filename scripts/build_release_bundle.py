#!/usr/bin/env python3

from __future__ import annotations

import argparse
import json
import tarfile
from collections import OrderedDict
from datetime import datetime, timezone
from pathlib import Path
from typing import NotRequired, TypeAlias, TypedDict, cast


JSONPrimitive: TypeAlias = str | int | float | bool | None
JSONValue: TypeAlias = JSONPrimitive | list["JSONValue"] | dict[str, "JSONValue"]
JSONDict: TypeAlias = dict[str, JSONValue]


class RuntimeTarget(TypedDict):
    name: str
    environment: str
    kind: str
    status: str
    sourcePaths: list[str]
    liveTarget: NotRequired[str]
    clusterTarget: NotRequired[str]


class ManifestBundle(TypedDict):
    name: str
    environment: str
    kind: str
    status: str
    liveTarget: str | None
    clusterTarget: str | None
    archiveName: str
    files: list[str]


class Manifest(TypedDict):
    repo: str
    generatedAtUtc: str
    bundles: list[ManifestBundle]


def load_contract(contract_path: Path) -> JSONDict:
    return cast(JSONDict, json.loads(contract_path.read_text(encoding="utf-8")))


def expand_entry(repo_root: Path, entry: str) -> list[Path]:
    if entry.endswith("/**"):
        prefix = repo_root / entry[:-3].rstrip("/")
        if not prefix.exists():
            return []
        return [path for path in prefix.rglob("*") if path.is_file()]

    path = repo_root / entry
    if path.is_dir():
        return [child for child in path.rglob("*") if child.is_file()]
    if path.is_file():
        return [path]
    return []


def collect_relative_paths(repo_root: Path, entries: list[str]) -> list[str]:
    ordered: OrderedDict[str, None] = OrderedDict()
    for entry in entries:
        for path in expand_entry(repo_root, entry):
            ordered[str(path.relative_to(repo_root)).replace("\\", "/")] = None
    return list(ordered.keys())


def build_manifest(repo_root: Path, contract: JSONDict) -> Manifest:
    bundles: list[ManifestBundle] = []
    deployment_model = cast(JSONDict, contract.get("deploymentModel", {}))
    runtime_targets = cast(list[RuntimeTarget], deployment_model.get("runtimeTargets", []))
    for target in runtime_targets:
        relative_paths = collect_relative_paths(repo_root, target.get("sourcePaths", []))
        bundles.append(
            {
                "name": target["name"],
                "environment": target["environment"],
                "kind": target["kind"],
                "status": target["status"],
                "liveTarget": target.get("liveTarget"),
                "clusterTarget": target.get("clusterTarget"),
                "archiveName": f"{target['name']}.tar.gz",
                "files": relative_paths,
            }
        )
    return {
        "repo": cast(str, contract["repo"]),
        "generatedAtUtc": datetime.now(timezone.utc).isoformat(),
        "bundles": bundles,
    }


def write_archive(repo_root: Path, relative_paths: list[str], archive_path: Path) -> None:
    archive_path.parent.mkdir(parents=True, exist_ok=True)
    with tarfile.open(archive_path, "w:gz") as tar:
        tar.add(repo_root / "deployment-contract.json", arcname="deployment-contract.json")
        for relative_path in relative_paths:
            tar.add(repo_root / relative_path, arcname=relative_path)


def build_release_bundle(repo_root: Path, output_dir: Path) -> Path:
    contract = load_contract(repo_root / "deployment-contract.json")
    manifest = build_manifest(repo_root, contract)
    output_dir.mkdir(parents=True, exist_ok=True)

    for bundle in manifest["bundles"]:
        write_archive(repo_root, bundle["files"], output_dir / bundle["archiveName"])

    manifest_path = output_dir / "release-bundles.json"
    _ = manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    return manifest_path


def main() -> int:
    parser = argparse.ArgumentParser(description="Build release bundle archives from deployment-contract.json.")
    _ = parser.add_argument("--output-dir", default="dist/release-bundles", help="Output directory for archives and manifest.")
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parents[1]
    manifest_path = build_release_bundle(repo_root, repo_root / cast(str, args.output_dir))
    print(f"release bundle manifest written to {manifest_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())