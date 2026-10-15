import importlib.util
import json
from pathlib import Path
import unittest
from unittest.mock import patch
import subprocess

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("resolve_images", ROOT / ".github/actions/tests/resolve-mcp-images/resolve_images.py")
images = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(images)
REGISTRY = "754489498669.dkr.ecr.eu-central-1.amazonaws.com"


class APIImagesTest(unittest.TestCase):
    def resolve(self, topology="mdcb"):
        calls = []
        def lookup(repository, ref, optional=False):
            calls.append((repository, ref, optional))
            if optional:
                return "c" * 40 if repository in ("tyk-analytics", "tyk-pro") else None
            return "b" * 40
        result = images.resolve("TykTechnologies/tyk-sink", "a" * 40, "feature-v2", "master", topology, lookup)
        return result, calls

    def test_resolves_caller_head_once_and_matches_peer_and_environment_branches(self):
        result, calls = self.resolve()
        self.assertEqual(result["tested_shas"], {"gateway": "b" * 40, "dashboard": "c" * 40,
                         "pump": "b" * 40, "sink": "a" * 40, "mock": images.MOCK_SHA})
        self.assertEqual(result["environment_sha"], "c" * 40)
        self.assertFalse(any(repo == "tyk-sink" for repo, _, _ in calls))
        direct, _ = self.resolve("direct")
        self.assertNotIn("sink", direct["tested_shas"])

    def test_freezes_every_image_after_peer_builds_finish(self):
        manifest, _ = self.resolve()
        attempts = {}
        ticks = [0]
        def lookup(repository, revision):
            attempts[repository] = attempts.get(repository, 0) + 1
            return None if attempts[repository] == 1 else "sha256:" + "d" * 64
        frozen = images.freeze(manifest, REGISTRY, lookup=lookup, timeout=30,
                               clock=lambda: ticks[0], sleep=lambda seconds: ticks.__setitem__(0, ticks[0] + seconds))
        self.assertEqual(set(frozen["images"]), set(manifest["tested_shas"]))
        self.assertTrue(all("@sha256:" in image for image in frozen["images"].values()))
        self.assertNotIn("images", manifest)
        self.assertTrue(all(count == 2 for count in attempts.values()))

    def test_missing_peer_image_fails_without_falling_back_to_mutable_tags(self):
        manifest, _ = self.resolve()
        ticks = [0]
        with self.assertRaisesRegex(RuntimeError, "not published"):
            images.freeze(manifest, REGISTRY, lookup=lambda *_: None, timeout=1,
                          clock=lambda: ticks[0], sleep=lambda seconds: ticks.__setitem__(0, ticks[0] + seconds))

    def test_invalid_source_registry_and_digest_are_rejected(self):
        manifest, _ = self.resolve()
        with self.assertRaises(ValueError):
            images.commit_sha("master")
        with self.assertRaises(ValueError):
            images.freeze(manifest, "arbitrary.registry", lookup=lambda *_: "sha256:" + "d" * 64)
        with self.assertRaises(ValueError):
            images.freeze(manifest, REGISTRY, lookup=lambda *_: "latest")
        with self.assertRaises(ValueError):
            images.resolve("TykTechnologies/portal", "a" * 40, "branch", "main", "direct")

    def test_registry_permission_errors_are_not_treated_as_missing_images(self):
        missing = subprocess.CompletedProcess([], 0, json.dumps({"images": [], "failures": [{"failureCode": "ImageNotFound"}]}), "")
        denied = subprocess.CompletedProcess([], 1, "", "An error occurred (AccessDeniedException) when calling BatchGetImage")
        with patch.object(images.subprocess, "run", return_value=missing):
            self.assertIsNone(images.image_digest("tyk-pump", "a" * 40))
        with patch.object(images.subprocess, "run", return_value=denied):
            with self.assertRaisesRegex(RuntimeError, "AccessDeniedException"):
                images.image_digest("tyk-pump", "a" * 40)

    def test_image_resolution_uses_existing_registry_pull_permission(self):
        found = subprocess.CompletedProcess([], 0, json.dumps({"images": [{"imageId": {"imageDigest": "sha256:" + "d" * 64}}], "failures": []}), "")
        with patch.object(images.subprocess, "run", return_value=found) as run:
            self.assertEqual(images.image_digest("tyk-pump", "a" * 40), "sha256:" + "d" * 64)
        self.assertEqual(run.call_args.args[0][:3], ["aws", "ecr", "batch-get-image"])
        invalid = subprocess.CompletedProcess([], 0, json.dumps({"images": [], "failures": [{"failureCode": "ImageTagDoesNotMatchDigest"}]}), "")
        with patch.object(images.subprocess, "run", return_value=invalid):
            with self.assertRaises(RuntimeError):
                images.image_digest("tyk-pump", "a" * 40)


if __name__ == "__main__":
    unittest.main()
