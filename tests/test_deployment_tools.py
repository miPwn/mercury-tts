from __future__ import annotations

import importlib.util
import json
import tarfile
import tempfile
import unittest
from pathlib import Path


def load_module(module_name: str, path: Path):
    spec = importlib.util.spec_from_file_location(module_name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class DeploymentToolsTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        repo_root = Path(__file__).resolve().parents[1]
        cls.validator = load_module("validator", repo_root / "scripts" / "validate_deployment_contract.py")
        cls.bundler = load_module("bundler", repo_root / "scripts" / "build_release_bundle.py")

    def test_validator_accepts_minimal_valid_contract(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            repo_root = Path(tmp)
            (repo_root / "alpha.txt").write_text("alpha\n", encoding="utf-8")
            (repo_root / "beta.txt").write_text("beta\n", encoding="utf-8")
            contract = {
                "repo": "sample",
                "deploymentModel": {
                    "runtimeTargets": [
                        {
                            "name": "falcon-target",
                            "environment": "falcon-host",
                            "kind": "script",
                            "status": "active",
                            "liveTarget": "/usr/local/bin/sample",
                            "sourcePaths": ["alpha.txt"],
                        }
                    ],
                    "integrationPoints": [
                        {
                            "name": "sample-http",
                            "targetRepo": "other",
                            "kind": "http",
                            "pathOrEndpoint": "/endpoint",
                            "sourcePaths": ["beta.txt"],
                        }
                    ],
                },
                "guardrails": {
                    "changeSets": [
                        {
                            "name": "touch-contract",
                            "triggers": ["alpha.txt"],
                            "requiresTouch": ["beta.txt"],
                        }
                    ]
                },
            }
            self.assertEqual(self.validator.validate_contract(contract, repo_root), [])

    def test_guardrail_reports_missing_required_touch(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            repo_root = Path(tmp)
            (repo_root / "deployment-contract.json").write_text("{}\n", encoding="utf-8")
            contract = {
                "guardrails": {
                    "changeSets": [
                        {
                            "name": "touch-contract",
                            "triggers": ["alpha.txt"],
                            "requiresTouch": ["deployment-contract.json"],
                        }
                    ]
                }
            }
            original = self.validator.get_changed_files
            self.validator.get_changed_files = lambda _repo_root, _base_ref: ["alpha.txt"]
            try:
                errors = self.validator.enforce_guardrails(contract, repo_root, "base")
            finally:
                self.validator.get_changed_files = original
            self.assertEqual(len(errors), 1)
            self.assertIn("deployment-contract.json", errors[0])

    def test_release_bundle_builds_archives_and_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            repo_root = Path(tmp)
            (repo_root / "folder").mkdir()
            (repo_root / "folder" / "nested.txt").write_text("nested\n", encoding="utf-8")
            (repo_root / "alpha.txt").write_text("alpha\n", encoding="utf-8")
            contract = {
                "repo": "sample",
                "deploymentModel": {
                    "runtimeTargets": [
                        {
                            "name": "bundle-one",
                            "environment": "falcon-host",
                            "kind": "script",
                            "status": "active",
                            "liveTarget": "/usr/local/bin/sample",
                            "sourcePaths": ["alpha.txt", "folder/**"],
                        }
                    ]
                },
            }
            (repo_root / "deployment-contract.json").write_text(json.dumps(contract), encoding="utf-8")
            manifest_path = self.bundler.build_release_bundle(repo_root, repo_root / "dist")
            manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
            self.assertEqual(manifest["repo"], "sample")
            archive_path = repo_root / "dist" / "bundle-one.tar.gz"
            self.assertTrue(archive_path.exists())
            with tarfile.open(archive_path, "r:gz") as tar:
                names = tar.getnames()
            self.assertIn("deployment-contract.json", names)
            self.assertIn("alpha.txt", names)
            self.assertIn("folder/nested.txt", names)


if __name__ == "__main__":
    unittest.main()