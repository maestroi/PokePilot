#!/usr/bin/env python3
"""Operator E2E: full game -> broadcast replay -> episode + Short -> optional private upload.

Standard-library-only client of the EXISTING private PokePilot APIs.
Without --upload-private it prepares publication candidates but NEVER uploads.
"""
import argparse
import json
import os
import re
import sys
import time
import uuid
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener


class E2EError(Exception):
    pass


class NoRedirects(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class API:
    def __init__(self, base, token=""):
        parsed = urlsplit(base)
        if parsed.scheme not in ("http", "https") or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise E2EError("Invalid private operator --base-url")
        self.base = base.rstrip("/")
        self.token = token
        self.opener = build_opener(NoRedirects())

    def call(self, method, route, data=None):
        body = json.dumps(data).encode() if data is not None else None
        request = Request(self.base + route, data=body, method=method)
        request.add_header("Accept", "application/json")
        if body is not None:
            request.add_header("Content-Type", "application/json")
        if self.token:
            request.add_header("Authorization", "Bearer " + self.token)
        try:
            with self.opener.open(request, timeout=40) as response:
                result = json.loads(response.read(2 << 20))
        except HTTPError as exc:
            try:
                detail = json.loads(exc.read(4096)).get("error", "request rejected")
            except (ValueError, AttributeError):
                detail = "request rejected"
            raise E2EError("HTTP %d: %s" % (exc.code, str(detail)[:180])) from None
        except (URLError, TimeoutError):
            raise E2EError("operator endpoint unavailable") from None
        if not isinstance(result, dict):
            raise E2EError("operator did not return a JSON object")
        return result


def digest(value, field):
    if not isinstance(value, str) or not re.fullmatch(r"[0-9a-f]{64}", value):
        raise E2EError("%s has no valid hash" % field)
    return value


def full_completion(response, game):
    run = response.get("run", response)
    stats, player = run.get("stats") or {}, run.get("player") or {}
    milestones = player.get("milestones") or []
    goal = str(run.get("goal") or "").strip().lower()
    hall = any("hall of fame" in str(m).lower() or "hall_of_fame" in str(m).lower() for m in milestones)
    return (run.get("game") == game and run.get("status") == "done"
            and run.get("reason") == "done"
            and goal in ("elite-four", "beat the elite four and champion.", "beat the elite four and champion")
            and (stats.get("goal_complete") is True or hall))


class Pipeline:
    def __init__(self, api, run_id, game, upload, poll, timeout):
        self.api, self.id, self.game = api, run_id, game
        self.upload, self.poll = upload, poll
        self.deadline = time.monotonic() + timeout
        self.report = {"run_id": run_id, "game": game, "uploads_requested": upload,
                       "state": "running", "steps": [], "outputs": {}}
        self.stage = "init"
        self.root = "/v1/runs/" + quote(run_id, safe="")

    def call(self, method, route, data=None):
        return self.api.call(method, route, data)

    def mark(self, name, **extra):
        self.stage = name
        self.report["steps"].append({"stage": name, **extra})
        print("[e2e]", name, flush=True)

    def await_state(self, name, path, ready, bad):
        self.stage = name
        while time.monotonic() < self.deadline:
            state = self.call("GET", path)
            if bad(state):
                raise E2EError("%s: %s" % (name, str(state.get("error") or state.get("reason") or state.get("state"))[:160]))
            if ready(state):
                self.mark(name, state="ready")
                return state
            time.sleep(min(self.poll, max(0, self.deadline - time.monotonic())))
        raise E2EError("timeout at " + name)

    def execute(self, launch, starter, seed, deployment):
        root = self.root
        try:
            if launch:
                spec = {"run_id": self.id, "game": self.game, "planner": "llm",
                        "starter": starter, "goal": "elite-four", "seed": seed,
                        "max_rounds": 0, "max_frames": 0, "recovery_profile": "resilient",
                        "endless": False, "random_seed": False}
                if deployment:
                    spec["llm_deployment"] = deployment
                self.call("POST", "/v1/specs", spec)
                self.mark("run_queued")
            result = self.await_state("run_settled", root,
                                      lambda r: r.get("run", r).get("status") == "done",
                                      lambda r: False)
            if not full_completion(result, self.game):
                raise E2EError("run did not prove a full Hall of Fame completion")
            self.mark("hall_of_fame_verified")
            if self.upload and self.call("GET", root + "/publish/youtube").get("configured") is not True:
                raise E2EError("YouTube uploader not configured")
            self.call("POST", root + "/replay/render?mode=broadcast", {})
            self.await_state("broadcast_render", root + "/replay/status?mode=broadcast",
                             lambda s: s.get("state") == "ready" and s.get("size", 1) > 0,
                             lambda s: s.get("state") in ("error", "failed", "cancelled"))
            assessment = self.call("POST", root + "/producer/assess", {})["assessment"]
            sha = digest(assessment.get("hash"), "assessment")
            verdict = assessment.get("publication_class")
            self.mark("producer_assessed", verdict=verdict)
            if verdict in ("archive_only", "do_not_publish"):
                archive = self.call("POST", root + "/studio/publications", {"assessment_hash": sha, "format": "archive"})
                self.report["outputs"]["archive"] = {"candidate_id": archive.get("id"), "state": archive.get("state")}
                raise E2EError("producer classified run as " + verdict + "; no upload permitted")
            if verdict not in ("full_episode", "short_only"):
                raise E2EError("unrecognized producer verdict")
            for fmt in (("episode", "short") if verdict == "full_episode" else ("short",)):
                plan = self.call("POST", root + "/studio/edits",
                                 {"assessment_hash": sha, "profile": fmt})["plan"]
                edit = root + "/studio/edits/" + digest(plan.get("hash"), "edit")
                self.call("POST", edit + "/render", {})
                self.await_state(fmt + "_render", edit + "/status",
                                 lambda s: s.get("state") == "ready",
                                 lambda s: s.get("state") in ("error", "failed", "cancelled"))
                package = self.call("POST", root + "/studio/packages",
                                    {"assessment_hash": sha, "edit_plan_hash": plan["hash"],
                                     "style": "documentary"})["package"]
                package_hash = digest(package.get("hash"), "package")
                thumbs = ""
                if fmt == "episode":
                    images_path = root + "/studio/packages/" + package_hash + "/thumbnails"
                    images = self.call("POST", images_path, {"variants": 1, "generated_art": False})
                    candidates = images.get("manifest", {}).get("candidates") or []
                    if not candidates:
                        raise E2EError("episode has no thumbnail")
                    thumbs = digest(candidates[0].get("id"), "thumbnail")
                    self.call("POST", images_path + "/" + thumbs + "/select", {})
                titles = package.get("titles" if fmt == "episode" else "shorts") or []
                if not titles or not titles[0].get("id"):
                    raise E2EError(fmt + " has no factual title")
                proposal = {"assessment_hash": sha, "package_hash": package_hash,
                            "format": fmt, "title_id": titles[0]["id"],
                            "privacy": "private", "not_before_unix": 0}
                if thumbs:
                    proposal["thumbnail_id"] = thumbs
                candidate = self.call("POST", root + "/studio/publications", proposal)
                cid = digest(candidate.get("id"), "publication")
                if candidate.get("state") not in ("queued", "approved", "published"):
                    raise E2EError(fmt + " blocked by quality gate: " + str(candidate.get("reason") or candidate.get("state")))
                output = {"plan_hash": plan["hash"], "package_hash": package_hash,
                          "candidate_id": cid, "privacy": "private", "state": candidate["state"]}
                self.report["outputs"][fmt] = output
                self.mark(fmt + "_prepared")
                if self.upload:
                    publication_path = root + "/studio/publications/" + cid
                    if candidate.get("state") != "published":
                        if candidate.get("state") != "approved":
                            candidate = self.call("POST", publication_path + "/approve", {})
                        if candidate.get("state") != "approved" or candidate.get("proposal", {}).get("privacy") != "private":
                            raise E2EError("publication is not explicitly approved as private")
                        while time.monotonic() < self.deadline:
                            try:
                                candidate = self.call("POST", publication_path + "/publish", {})
                                break
                            except E2EError as exc:
                                if "channel minimum upload interval" not in str(exc):
                                    raise
                                time.sleep(min(self.poll, max(0, self.deadline - time.monotonic())))
                        else:
                            raise E2EError("timed out waiting for YouTube upload slot")
                    def read_publication():
                        for entry in self.call("GET", root + "/studio/publications").get("candidates", []):
                            if entry.get("id") == cid:
                                return entry
                        raise E2EError("publication missing from durable queue")
                    self.stage = fmt + "_private_upload"
                    while time.monotonic() < self.deadline:
                        current = read_publication()
                        if current.get("state") in ("failed", "blocked", "skipped"):
                            raise E2EError("upload failed: " + str(current.get("reason") or current.get("state")))
                        if current.get("state") == "published":
                            if current.get("proposal", {}).get("privacy") != "private" or not current.get("provider_id"):
                                raise E2EError("publication missing private provenance or YouTube video ID")
                            output.update(state="published", video_id=current["provider_id"],
                                          video_url=current.get("provider_url"))
                            self.mark(fmt + "_private_upload")
                            break
                        time.sleep(min(self.poll, max(0, self.deadline - time.monotonic())))
                    else:
                        raise E2EError("upload timeout")
            if verdict != "full_episode":
                raise E2EError("Short-only classification cannot pass full episode + Short test")
            self.report["state"] = "passed"
        except (E2EError, KeyError, TypeError, ValueError) as exc:
            self.report.update(state="failed", failed_stage=self.stage, error=str(exc)[:240])
            raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True, help="private pokeui URL")
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--launch", action="store_true")
    source.add_argument("--run-id")
    parser.add_argument("--game", choices=("pokemon-red", "pokemon-yellow"), default="pokemon-red")
    parser.add_argument("--starter", default="squirtle")
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument("--deployment", default="")
    parser.add_argument("--upload-private", action="store_true")
    parser.add_argument("--poll-seconds", type=float, default=20)
    parser.add_argument("--timeout-seconds", type=float, default=48 * 3600)
    parser.add_argument("--token-env", default="POKEPILOT_E2E_TOKEN")
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    if args.poll_seconds <= 0 or args.timeout_seconds <= 0:
        parser.error("poll and timeout must be positive")
    if args.game == "pokemon-yellow" and args.starter != "pikachu":
        parser.error("Yellow requires --starter pikachu")
    ident = args.run_id or "e2e-" + uuid.uuid4().hex[:16]
    workflow = None
    try:
        workflow = Pipeline(API(args.base_url, os.environ.get(args.token_env, "")),
                            ident, args.game, args.upload_private,
                            args.poll_seconds, args.timeout_seconds)
        workflow.execute(args.launch, args.starter, args.seed, args.deployment)
        print("[e2e] PASS", ident)
        return 0
    except (E2EError, KeyError, ValueError) as exc:
        print("[e2e] FAIL:", exc, file=sys.stderr)
        return 1
    finally:
        if workflow and args.report:
            args.report.parent.mkdir(parents=True, exist_ok=True)
            args.report.write_text(json.dumps(workflow.report, indent=2) + "\n")


if __name__ == "__main__":
    sys.exit(main())
