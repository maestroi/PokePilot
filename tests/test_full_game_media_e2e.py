"""ROM-free contract tests; real ROM/YouTube proof belongs in the private E2E lane."""
import unittest

from tools.full_game_media_e2e import API, E2EError, Pipeline, full_completion


class StubAPI:
    def __init__(self, verdict="full_episode"):
        self.verdict = verdict
        self.calls = []
        self.uploads = {}
        self.proposals = []
        self.private_requests = []

    def call(self, method, path, data=None):
        self.calls.append((method, path, data))
        if path == "/v1/specs":
            return {"status": "queued"}
        if path.endswith("/publish/youtube"):
            return {"configured": True}
        if path == "/v1/runs/test" and method == "GET":
            return {"run": {"run_id": "test", "game": "pokemon-red", "status": "done",
                            "reason": "done", "goal": "elite-four",
                            "stats": {"goal_complete": True}}}
        if "/replay/render" in path:
            return {"state": "generating"}
        if "/replay/status" in path:
            return {"state": "ready", "size": 100}
        if path.endswith("/producer/assess"):
            return {"assessment": {"hash": "a" * 64, "publication_class": self.verdict}}
        if path.endswith("/studio/edits") and method == "POST":
            return {"plan": {"hash": ("b" if data["profile"] == "episode" else "c") * 64}}
        if path.endswith("/render"):
            return {"state": "generating"}
        if path.endswith("/status"):
            return {"state": "ready"}
        if path.endswith("/studio/packages") and method == "POST":
            key = "d" if data["edit_plan_hash"] == "b" * 64 else "e"
            return {"package": {"hash": key * 64, "titles": [{"id": "title-1"}],
                                "shorts": [{"id": "short-1"}]}}
        if path.endswith("/thumbnails") and method == "POST":
            return {"manifest": {"candidates": [{"id": "f" * 64}]}}
        if path.endswith("/select"):
            return {"candidate_id": "f" * 64}
        if path.endswith("/studio/publications"):
            if method == "GET":
                return {"candidates": list(self.uploads.values())}
            self.proposals.append(data)
            if data["format"] == "archive":
                return {"id": "0" * 64, "state": "skipped"}
            key = ("1" if data["format"] == "episode" else "2") * 64
            candidate = {"id": key, "state": "queued", "proposal": data}
            self.uploads[key] = candidate
            return candidate
        if "/studio/publications/" in path:
            key = path.split("/studio/publications/")[1].split("/")[0]
            c = self.uploads[key]
            if path.endswith("/approve"):
                c["state"] = "approved"
                return c
            if path.endswith("/publish"):
                self.private_requests.append(c["proposal"]["privacy"])
                c["state"] = "published"
                c["provider_id"] = "youtube-" + key[0]
                c["provider_url"] = "https://youtu.be/" + key[0]
                return c
        raise AssertionError("unexpected endpoint: %s %s" % (method, path))


def execute(upload=False, verdict="full_episode"):
    fake = StubAPI(verdict)
    pipeline = Pipeline(fake, "test", "pokemon-red", upload, poll=0.01, timeout=5)
    pipeline.execute(launch=True, starter="squirtle", seed=1, deployment="")
    return fake, pipeline


class E2EContractTests(unittest.TestCase):
    def test_qualification_requires_semantic_proof(self):
        run = {"run": {"game": "pokemon-red", "status": "done",
                       "reason": "done", "goal": "elite-four", "stats": {}}}
        self.assertFalse(full_completion(run, "pokemon-red"))
        run["run"]["stats"]["goal_complete"] = True
        self.assertTrue(full_completion(run, "pokemon-red"))
        run["run"]["reason"] = "budget"
        self.assertFalse(full_completion(run, "pokemon-red"))

    def test_dry_run_creates_two_private_candidates_without_upload(self):
        fake, runner = execute()
        self.assertEqual(runner.report["state"], "passed")
        self.assertEqual(set(runner.report["outputs"]), {"episode", "short"})
        self.assertEqual(fake.private_requests, [])
        self.assertEqual(len(fake.proposals), 2)
        self.assertTrue(all(p["privacy"] == "private" for p in fake.proposals))
        self.assertFalse(any(path.endswith("/approve") or path.endswith("/publish")
                             for _, path, _ in fake.calls))

    def test_explicit_upload_only_private_for_both_formats(self):
        fake, runner = execute(upload=True)
        self.assertEqual(runner.report["state"], "passed")
        self.assertEqual(fake.private_requests, ["private", "private"])
        self.assertNotEqual(runner.report["outputs"]["episode"]["video_id"],
                            runner.report["outputs"]["short"]["video_id"])

    def test_archive_verdict_cannot_upload(self):
        fake = StubAPI("archive_only")
        runner = Pipeline(fake, "test", "pokemon-red", True, poll=0.01, timeout=5)
        with self.assertRaisesRegex(E2EError, "no upload permitted"):
            runner.execute(False, "squirtle", 1, "")
        self.assertEqual(runner.report["state"], "failed")
        self.assertEqual(fake.private_requests, [])
        self.assertIn("archive", runner.report["outputs"])

    def test_short_only_never_claims_full_e2e_success(self):
        fake = StubAPI("short_only")
        runner = Pipeline(fake, "test", "pokemon-red", False, poll=0.01, timeout=5)
        with self.assertRaisesRegex(E2EError, "Short-only"):
            runner.execute(False, "squirtle", 1, "")
        self.assertEqual(runner.report["state"], "failed")
        self.assertNotIn("episode", runner.report["outputs"])

    def test_operator_url_rejects_embedded_credentials(self):
        with self.assertRaises(E2EError):
            API("https://test:secret@example.org")

    def test_report_has_source_and_output_identifiers(self):
        _, runner = execute()
        self.assertEqual(runner.report["run_id"], "test")
        self.assertEqual(len(runner.report["outputs"]["episode"]["plan_hash"]), 64)
        self.assertEqual(len(runner.report["outputs"]["short"]["package_hash"]), 64)


if __name__ == "__main__":
    unittest.main()
