#!/usr/bin/env python3
"""ROM-free held-out response benchmark against one OpenAI-compatible model.

This measures format/legality/teacher agreement, NOT Pokémon progression.
Compare production runs with existing pokebench or paired experiments.
"""
from __future__ import annotations

import argparse
from collections import Counter
import json
import os
from pathlib import Path
import re
import statistics
import sys
import time
from urllib import error, request
from urllib.parse import urlsplit


def offered_labels(user_prompt: str) -> list[str]:
    # PokePilot numbers offered objectives at the start of lines. Notes are
    # appended after a double-space; these are not part of objective identity.
    result = []
    in_menu = False
    for line in user_prompt.splitlines():
        if line.startswith("Offered objectives"):
            in_menu = True
            continue
        if not in_menu:
            continue
        found = re.match(r"^(\d+): (.+)$", line)
        if found and int(found.group(1)) == len(result) + 1:
            result.append(found.group(2).split("  ", 1)[0])
    return result


def parse_response(task: str, content: str, offered: list[str]):
    try:
        obj = json.loads(content)
    except (ValueError, TypeError):
        return "invalid_json", None
    if not isinstance(obj, dict):
        return "invalid_shape", None
    if task == "objective_choice":
        choice = obj.get("choice")
        if type(choice) is not int or choice < 1 or choice > len(offered):
            return "illegal_choice", None
        if "intent" in obj and (not isinstance(obj["intent"], str)
                                or len(obj["intent"].encode("utf-8")) > 200):
            return "invalid_intent", None
        return "valid", {"choice": choice}
    if task == "strategic_plan":
        goal, steps = obj.get("goal"), obj.get("steps")
        if not isinstance(goal, str) or not goal.strip() or not isinstance(steps, list):
            return "invalid_shape", None
        if not 1 <= len(steps) <= 10 or any(
            not isinstance(s, str) or s not in offered for s in steps
        ):
            return "illegal_plan", None
        return "valid", {"goal": goal, "steps": steps}
    return "unknown_task", None


def post_model(endpoint: str, model: str, messages: list[dict], task: str,
               timeout: float, token: str) -> tuple[str, float, dict]:
    plan = task == "strategic_plan"
    schema = {
        "type": "object", "properties": {
            "goal": {"type": "string"},
            "steps": {"type": "array", "items": {"type": "string"}, "minItems": 1, "maxItems": 10},
        }, "required": ["goal", "steps"],
    } if plan else {
        "type": "object", "properties": {
            "choice": {"type": "integer"},
            "intent": {"type": "string"},
        }, "required": ["choice"],
    }
    body = json.dumps({
        "model": model, "messages": messages, "temperature": 0,
        "max_tokens": 2048 if plan else 256,
        "response_format": {
            "type": "json_schema",
            "json_schema": {
                "name": "objective_plan" if plan else "objective_choice",
                "strict": False, "schema": schema,
            },
        },
    }).encode("utf-8")
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    url = endpoint.rstrip("/") + "/chat/completions"
    start = time.monotonic()
    with request.urlopen(request.Request(url, data=body, headers=headers), timeout=timeout) as response:
        result = json.load(response)
    seconds = time.monotonic() - start
    if not result.get("choices"):
        raise ValueError("inference response has no choices")
    content = result["choices"][0]["message"].get("content", "")
    if result["choices"][0].get("finish_reason") not in (None, "stop"):
        return "", seconds, {"finish_reason": result["choices"][0].get("finish_reason")}
    return content, seconds, result.get("usage", {})


def benchmark(dataset: Path, endpoint: str, model: str, timeout: float,
              limit: int, token: str) -> dict:
    test_file = dataset / "test.jsonl"
    if not test_file.is_file():
        raise ValueError(f"missing test set: {test_file}")
    counters: Counter[str] = Counter()
    latencies: list[float] = []
    total_tokens = 0
    with test_file.open(encoding="utf-8") as stream:
        for line in stream:
            if not line.strip():
                continue
            sample = json.loads(line)
            prompt, completion = sample["prompt"], sample["completion"]
            target = json.loads(completion[0]["content"])
            task = "objective_choice" if "choice" in target else "strategic_plan"
            labels = offered_labels(prompt[1]["content"])
            if not labels:
                raise ValueError("test sample is missing the exact PokePilot objective menu")
            if limit > 0 and counters["total"] >= limit:
                break
            counters["total"] += 1
            try:
                content, seconds, usage = post_model(endpoint, model, prompt, task, timeout, token)
            except (error.URLError, ValueError, TimeoutError, KeyError) as exc:
                counters["transport_or_envelope_error"] += 1
                continue
            latencies.append(seconds)
            total_tokens += usage.get("completion_tokens", 0)
            if not content:
                counters["truncated"] += 1
                continue
            status, pred = parse_response(task, content, labels)
            counters[status] += 1
            if status == "valid":
                # Teacher agreement is a proxy, never a claim of optimality.
                comparable = {"choice": target["choice"]} if task == "objective_choice" else target
                if pred == comparable:
                    counters["agreed"] += 1
            counters[f"task_{task}"] += 1
    if not counters["total"]:
        raise ValueError("held-out test set is empty; gather more distinct run seeds")
    n = counters["total"]
    return {
        "model": model,
        "endpoint_host": urlsplit(endpoint).hostname,
        "samples": n,
        "valid_rate": counters["valid"] / n,
        "teacher_agreement_rate": counters["agreed"] / n,
        "errors": dict(counters),
        "average_latency_seconds": statistics.mean(latencies) if latencies else None,
        "completion_tokens": total_tokens,
        "disclaimer": "Offline teacher agreement, NOT validated game progression. Use paired farm runs before promotion.",
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dataset", type=Path, required=True)
    parser.add_argument("--endpoint", required=True,
                        help="explicit OpenAI-compatible /v1 URL; sends private game observations")
    parser.add_argument("--model", required=True, help="served model identifier")
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--timeout", type=float, default=60)
    parser.add_argument("--limit", type=int, default=0)
    args = parser.parse_args(argv)
    if args.timeout <= 0 or args.limit < 0:
        parser.error("timeout must be positive and limit nonnegative")
    if urlsplit(args.endpoint).scheme not in ("http", "https"):
        parser.error("endpoint must be an explicit http(s) URL")
    try:
        report = benchmark(args.dataset, args.endpoint, args.model, args.timeout,
                           args.limit, os.environ.get("POKEPILOT_TRAIN_EVAL_TOKEN", ""))
    except (OSError, ValueError, KeyError) as exc:
        parser.error(str(exc))
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps(report, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
