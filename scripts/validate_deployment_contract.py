#!/usr/bin/env python3

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path


ALLOWED_ENVIRONMENTS = {
    "falcon-host",
    "falcon-k3s",
    "archos-host",
    "windows-client",
    "shared-contract",
}
ALLOWED_TARGET_KINDS = {
    "api",
    "binary",
    "display",
    "k8s",
    "script",
    "service",
    "wrapper",
}
ALLOWED_TARGET_STATUSES = {"active", "external", "legacy", "manual", "secondary"}
ALLOWED_INTEGRATION_KINDS = {"command", "filesystem", "http", "queue", "state"}


def load_contract(contract_path: Path) -> dict:
    try:
        return json.loads(contract_path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise SystemExit(f"deployment contract not found: {contract_path}") from exc
    except json.JSONDecodeError as exc:
        raise SystemExit(f"invalid JSON in {contract_path}: {exc}") from exc


def is_pattern(path_text: str) -> bool:
    return path_text.endswith("/**")


def validate_repo_path(repo_root: Path, entry: str, errors: list[str], label: str) -> None:
    if is_pattern(entry):
        prefix = entry[:-3].rstrip("/")
        if not prefix:
            errors.append(f"{label}: invalid wildcard pattern {entry}")
            return
        if not (repo_root / prefix).exists():
            errors.append(f"{label}: wildcard prefix does not exist: {entry}")
        return

    if not (repo_root / entry).exists():
        errors.append(f"{label}: referenced path does not exist: {entry}")


def require_keys(node: dict, required: list[str], errors: list[str], label: str) -> None:
    for key in required:
        if key not in node:
            errors.append(f"{label}: missing required key '{key}'")


def matches_pattern(path_text: str, pattern: str) -> bool:
    normalized_path = path_text.replace("\\", "/")
    normalized_pattern = pattern.replace("\\", "/")
    if normalized_pattern.endswith("/**"):
        prefix = normalized_pattern[:-3].rstrip("/")
        return normalized_path == prefix or normalized_path.startswith(prefix + "/")
    return normalized_path == normalized_pattern


def get_changed_files(repo_root: Path, base_ref: str) -> list[str]:
    if not base_ref or set(base_ref) == {"0"}:
        return []

    commands = [
        ["git", "-C", str(repo_root), "diff", "--name-only", f"{base_ref}...HEAD"],
        ["git", "-C", str(repo_root), "diff", "--name-only", base_ref, "HEAD"],
    ]
    for command in commands:
        result = subprocess.run(command, capture_output=True, text=True, check=False)
        if result.returncode == 0:
            return [line.strip().replace("\\", "/") for line in result.stdout.splitlines() if line.strip()]
    return []


def validate_contract(contract: dict, repo_root: Path) -> list[str]:
    errors: list[str] = []

    require_keys(contract, ["repo", "deploymentModel", "guardrails"], errors, "root")
    deployment_model = contract.get("deploymentModel", {})
    guardrails = contract.get("guardrails", {})
    require_keys(deployment_model, ["runtimeTargets", "integrationPoints"], errors, "deploymentModel")
    require_keys(guardrails, ["changeSets"], errors, "guardrails")

    runtime_targets = deployment_model.get("runtimeTargets", [])
    if not isinstance(runtime_targets, list) or not runtime_targets:
        errors.append("deploymentModel.runtimeTargets: must be a non-empty list")
    else:
        for index, target in enumerate(runtime_targets, start=1):
            label = f"runtimeTargets[{index}]"
            if not isinstance(target, dict):
                errors.append(f"{label}: must be an object")
                continue
            require_keys(target, ["name", "environment", "kind", "status", "sourcePaths"], errors, label)
            if target.get("environment") not in ALLOWED_ENVIRONMENTS:
                errors.append(f"{label}: invalid environment '{target.get('environment')}'")
            if target.get("kind") not in ALLOWED_TARGET_KINDS:
                errors.append(f"{label}: invalid kind '{target.get('kind')}'")
            if target.get("status") not in ALLOWED_TARGET_STATUSES:
                errors.append(f"{label}: invalid status '{target.get('status')}'")
            if not target.get("liveTarget") and not target.get("clusterTarget"):
                errors.append(f"{label}: must define liveTarget or clusterTarget")
            source_paths = target.get("sourcePaths", [])
            if not isinstance(source_paths, list) or not source_paths:
                errors.append(f"{label}: sourcePaths must be a non-empty list")
            else:
                for source_path in source_paths:
                    validate_repo_path(repo_root, source_path, errors, f"{label}.sourcePaths")

    integration_points = deployment_model.get("integrationPoints", [])
    if not isinstance(integration_points, list) or not integration_points:
        errors.append("deploymentModel.integrationPoints: must be a non-empty list")
    else:
        for index, point in enumerate(integration_points, start=1):
            label = f"integrationPoints[{index}]"
            if not isinstance(point, dict):
                errors.append(f"{label}: must be an object")
                continue
            require_keys(point, ["name", "targetRepo", "kind", "pathOrEndpoint", "sourcePaths"], errors, label)
            if point.get("kind") not in ALLOWED_INTEGRATION_KINDS:
                errors.append(f"{label}: invalid kind '{point.get('kind')}'")
            source_paths = point.get("sourcePaths", [])
            if not isinstance(source_paths, list) or not source_paths:
                errors.append(f"{label}: sourcePaths must be a non-empty list")
            else:
                for source_path in source_paths:
                    validate_repo_path(repo_root, source_path, errors, f"{label}.sourcePaths")

    change_sets = guardrails.get("changeSets", [])
    if not isinstance(change_sets, list) or not change_sets:
        errors.append("guardrails.changeSets: must be a non-empty list")
    else:
        for index, change_set in enumerate(change_sets, start=1):
            label = f"changeSets[{index}]"
            if not isinstance(change_set, dict):
                errors.append(f"{label}: must be an object")
                continue
            require_keys(change_set, ["name", "triggers", "requiresTouch"], errors, label)
            triggers = change_set.get("triggers", [])
            requires_touch = change_set.get("requiresTouch", [])
            if not isinstance(triggers, list) or not triggers:
                errors.append(f"{label}: triggers must be a non-empty list")
            else:
                for trigger in triggers:
                    validate_repo_path(repo_root, trigger, errors, f"{label}.triggers")
            if not isinstance(requires_touch, list) or not requires_touch:
                errors.append(f"{label}: requiresTouch must be a non-empty list")
            else:
                for touch_path in requires_touch:
                    validate_repo_path(repo_root, touch_path, errors, f"{label}.requiresTouch")

    return errors


def enforce_guardrails(contract: dict, repo_root: Path, base_ref: str) -> list[str]:
    changed_files = set(get_changed_files(repo_root, base_ref))
    if not changed_files:
        return []

    errors: list[str] = []
    for change_set in contract.get("guardrails", {}).get("changeSets", []):
        triggers = change_set.get("triggers", [])
        requires_touch = change_set.get("requiresTouch", [])
        triggered_files = sorted(
            changed_file
            for changed_file in changed_files
            if any(matches_pattern(changed_file, trigger) for trigger in triggers)
        )
        if not triggered_files:
            continue
        missing_touches = [path for path in requires_touch if path.replace("\\", "/") not in changed_files]
        if missing_touches:
            errors.append(
                f"guardrail '{change_set.get('name', 'unnamed')}' triggered by {', '.join(triggered_files)} but missing required review file updates: {', '.join(missing_touches)}"
            )
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description="Validate deployment-contract.json and change guardrails.")
    parser.add_argument("--base-ref", default="", help="Git base ref or SHA used for changed-file guardrails.")
    parser.add_argument("--check-changed", action="store_true", help="Enforce guardrails for files changed since --base-ref.")
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parents[1]
    contract_path = repo_root / "deployment-contract.json"
    contract = load_contract(contract_path)

    errors = validate_contract(contract, repo_root)
    if args.check_changed:
        errors.extend(enforce_guardrails(contract, repo_root, args.base_ref))

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print(f"deployment contract validation passed for {contract.get('repo', 'unknown-repo')}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())