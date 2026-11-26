#!/usr/bin/env python3
"""Turn PokePilot's opt-in raw planner traces into conservative SFT examples.

No external dependencies. This command never calls inference or trains a model.
"""
from __future__ import annotations

import argparse
from collections import Counter
import gzip
import hashlib
import json
from pathlib import Path
import sys

CORPUS_NAME = "planner-training-v1.jsonl.gz"
TASKS = {"objective_choice", "strategic_plan"}
SPLITS = ("train", "validation", "test")


def sources(paths: list[str]) -> list[Path]:
    found: set[Path] = set()
    for arg in paths:
        item = Path(arg)
        if item.is_file():
            found.add(item.resolve())
        elif item.is_dir():
            found.update(p.resolve() for p in item.rglob(CORPUS_NAME) if p.is_file())
        else:
            raise ValueError(f"input not found: {item}")
    if not found:
        raise ValueError("no planner-training-v1.jsonl.gz files found")
    return sorted(found)


def read_records(path: Path):
    opener = gzip.open if path.suffix == ".gz" else open
    with opener(path, "rt", encoding="utf-8") as stream:
        for line_number, line in enumerate(stream, 1):
            if not line.strip():
                continue
            try:
                obj = json.loads(line)
            except (json.JSONDecodeError, UnicodeError) as exc:
                raise ValueError(f"{path}:{line_number}: invalid JSON: {exc}") from exc
            if not isinstance(obj, dict):
                raise ValueError(f"{path}:{line_number}: expected a JSON object")
            yield obj


def good_text(value) -> bool:
    return isinstance(value, str) and bool(value.strip())


def candidate(raw: dict, *, allow_multistep: bool = False):
    """Return (reason, sample, provenance) where reason is 'accepted' or why filtered."""
    if raw.get("version") != 1:
        return "schema_version", None, None
    if raw.get("game") != "pokemon-red":
        return "not_gen1_red", None, None
    task = raw.get("task")
    if task not in TASKS:
        return "unknown_task", None, None
    if not good_text(raw.get("run_id")) or type(raw.get("seed")) is not int:
        return "missing_identity", None, None
    if not good_text(raw.get("rom_sha256")) or not good_text(raw.get("prompt_hash")):
        return "missing_provenance", None, None
    if not good_text(raw.get("system_prompt")) or not good_text(raw.get("user_prompt")):
        return "missing_exact_prompts", None, None
    offered = raw.get("offered")
    if not isinstance(offered, list) or not offered or not all(good_text(o) for o in offered):
        return "bad_menu", None, None
    if raw.get("rejected") or raw.get("error"):
        return "rejected", None, None
    execution = raw.get("execution")
    if not isinstance(execution, dict):
        return "unverified_execution", None, None
    if (execution.get("outcome") != "completed"
            or execution.get("terminal", False)
            or execution.get("recovered", False)):
        return "unsuccessful_execution", None, None
    if (type(raw.get("round")) is not int or execution.get("round") != raw["round"]
            or not good_text(execution.get("objective"))):
        return "unmatched_execution", None, None
    before, after = execution.get("badges_before"), execution.get("badges_after")
    if (type(before) is not int or type(after) is not int
            or before < 0 or after < before):
        return "invalid_progress", None, None

    if task == "objective_choice":
        choice = raw.get("choice")
        if not good_text(choice) or offered.count(choice) != 1:
            return "invalid_choice", None, None
        if choice != execution["objective"]:
            return "unmatched_execution", None, None
        reply = {"choice": offered.index(choice) + 1}
        intent = raw.get("choice_intent")
        if good_text(intent) and len(intent.encode("utf-8")) <= 200:
            reply["intent"] = intent
    else:
        goal = raw.get("plan_goal")
        steps = raw.get("plan_steps")
        if not good_text(goal) or not isinstance(steps, list) or not steps:
            return "invalid_plan", None, None
        if len(steps) > 10 or not all(good_text(s) and s in offered for s in steps):
            return "invalid_plan", None, None
        # Later plan steps do not have linked outcomes in corpus v1. Default
        # to one-step legs, so the example is not labeled successful because
        # only the first step happened to work.
        if len(steps) > 1 and not allow_multistep:
            return "unverified_plan_tail", None, None
        if steps[0] != execution["objective"]:
            return "unmatched_execution", None, None
        reply = {"goal": goal, "steps": steps}

    prompt = [
        {"role": "system", "content": raw["system_prompt"]},
        {"role": "user", "content": raw["user_prompt"]},
    ]
    sample = {
        "prompt": prompt,
        "completion": [{"role": "assistant", "content": json.dumps(reply, ensure_ascii=False, separators=(",", ":"))}],
    }
    identity = {
        "task": task, "game": raw["game"], "seed": raw["seed"],
        "rom_sha256": raw["rom_sha256"], "run_id": raw["run_id"],
        "attempt": raw.get("attempt"), "prompt_hash": raw["prompt_hash"],
    }
    return "accepted", sample, identity


def sample_fingerprint(sample: dict) -> str:
    # Dedupe by model input, not the model's output: conflicting decisions for
    # identical observations must not silently become contradictory labels.
    msg = json.dumps(sample["prompt"], ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(msg.encode("utf-8")).hexdigest()


def split_for(identity: dict, salt: str, validation_pct: int, test_pct: int) -> str:
    # Split by game+ROM+seed, never by individual decision or attempt.
    key = f'{salt}|{identity["game"]}|{identity["rom_sha256"]}|{identity["seed"]}'
    bucket = int.from_bytes(hashlib.sha256(key.encode("utf-8")).digest()[:8], "big") % 100
    if bucket < test_pct:
        return "test"
    if bucket < test_pct + validation_pct:
        return "validation"
    return "train"


def write_jsonl(path: Path, samples: list[dict]) -> None:
    tmp = path.with_name(path.name + ".tmp")
    with tmp.open("w", encoding="utf-8", newline="\n") as out:
        for example in samples:
            out.write(json.dumps(example, ensure_ascii=False, separators=(",", ":")) + "\n")
    tmp.replace(path)


def prepare(inputs: list[str], output: Path, *, task: str = "all",
            validation_pct: int = 10, test_pct: int = 10, salt: str = "pokepilot-sft-v1",
            allow_multistep: bool = False) -> dict:
    if task not in TASKS | {"all"}:
        raise ValueError(f"unknown task {task!r}")
    if not (0 <= test_pct <= 50 and 0 <= validation_pct <= 50 and
            test_pct + validation_pct < 100):
        raise ValueError("validation/test percentages must be 0..50 and sum below 100")
    if not salt:
        raise ValueError("split salt must not be empty")
    paths = sources(inputs)
    rejections: Counter[str] = Counter()
    # Keep the first copy of an input and reject *all* variants if its
    # outputs conflict. This also removes cross-seed duplicate prompt leakage.
    chosen: dict[str, tuple[dict, dict]] = {}
    conflicts: set[str] = set()
    for path in paths:
        for raw in read_records(path):
            if task != "all" and raw.get("task") != task:
                rejections["different_task"] += 1
                continue
            reason, sample, identity = candidate(raw, allow_multistep=allow_multistep)
            if reason != "accepted":
                rejections[reason] += 1
                continue
            fp = sample_fingerprint(sample)
            if fp in conflicts:
                rejections["conflicting_duplicate"] += 1
                continue
            if fp in chosen:
                prior, _ = chosen[fp]
                if prior["completion"] != sample["completion"]:
                    del chosen[fp]
                    conflicts.add(fp)
                    rejections["conflicting_duplicate"] += 2
                else:
                    rejections["duplicate_input"] += 1
                continue
            chosen[fp] = sample, identity

    outputs: dict[str, list[dict]] = {split: [] for split in SPLITS}
    by_task: dict[str, Counter[str]] = {split: Counter() for split in SPLITS}
    # Sort by fingerprint rather than input file traversal for reproducibility.
    for fp in sorted(chosen):
        sample, identity = chosen[fp]
        split = split_for(identity, salt, validation_pct, test_pct)
        outputs[split].append(sample)
        by_task[split][identity["task"]] += 1
    if not outputs["train"]:
        raise ValueError("no trainable examples in the training split; gather more opted-in runs")
    output.mkdir(parents=True, exist_ok=True)
    for split in SPLITS:
        write_jsonl(output / f"{split}.jsonl", outputs[split])
    manifest = {
        "version": 1,
        "format": "trl-conversational-prompt-completion",
        "filter": "direct completed unrecovered nonterminal execution",
        "multistep": allow_multistep,
        "source_files": [str(path) for path in paths],
        "split_policy": "sha256(game, rom_sha256, seed, salt) plus global input deduplication",
        "split_salt": salt, "validation_pct": validation_pct, "test_pct": test_pct,
        "counts": {split: len(outputs[split]) for split in SPLITS},
        "tasks": {split: dict(by_task[split]) for split in SPLITS},
        "rejected": dict(sorted(rejections.items())),
    }
    (output / "manifest.json").write_text(
        json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    return manifest


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("inputs", nargs="+", help="corpus files or directories containing run corpora")
    parser.add_argument("--out", type=Path, required=True, help="destination dataset directory")
    parser.add_argument("--task", choices=["all", *sorted(TASKS)], default="all")
    parser.add_argument("--validation-pct", type=int, default=10)
    parser.add_argument("--test-pct", type=int, default=10)
    parser.add_argument("--split-salt", default="pokepilot-sft-v1")
    parser.add_argument("--allow-multistep", action="store_true",
                        help="experimental: train multi-step plans with only first-step success evidence")
    args = parser.parse_args(argv)
    try:
        manifest = prepare(args.inputs, args.out, task=args.task,
                           validation_pct=args.validation_pct, test_pct=args.test_pct,
                           salt=args.split_salt, allow_multistep=args.allow_multistep)
    except (OSError, ValueError, EOFError) as exc:
        parser.error(str(exc))
    print(json.dumps({"counts": manifest["counts"], "rejected": manifest["rejected"]}, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
