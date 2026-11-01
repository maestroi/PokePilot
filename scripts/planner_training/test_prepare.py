"""ROM-free, dependency-free tests for the planner SFT exporter."""
from __future__ import annotations

import gzip
import json
from pathlib import Path
import tempfile
import unittest

from prepare import candidate, prepare, split_for


def example(*, seed: int = 3, run: str = "run-a", task: str = "objective_choice",
            choice: str = "heal your party", prompt: str = "unique prompt",
            round_no: int = 5) -> dict:
    value = {
        "version": 1, "run_id": run, "attempt": 1, "game": "pokemon-red",
        "seed": seed, "rom_sha256": "rom", "build": "main", "prompt_hash": "hash",
        "task": task, "round": round_no, "rejected": False, "goal": "badges:1",
        "system_prompt": "exact strategist system" if task == "strategic_plan" else "exact chooser system",
        "user_prompt": prompt, "offered": ["heal your party", "beat the gym leader here"],
        "choice": choice, "choice_intent": "Prepare for the next badge.",
        "execution": {
            "round": round_no, "objective": choice, "outcome": "completed",
            "badges_before": 0, "badges_after": 0, "recovered": False,
        },
    }
    if task == "strategic_plan":
        value.update({
            "plan_goal": "earn the badge", "plan_steps": [choice],
            "replan_reason": "initial",
        })
    return value


def corpus(folder: Path, rows: list[dict], filename: str = "planner-training-v1.jsonl.gz") -> Path:
    folder.mkdir(parents=True, exist_ok=True)
    target = folder / filename
    with gzip.open(target, "wt", encoding="utf-8") as stream:
        for row in rows:
            stream.write(json.dumps(row) + "\n")
    return target


class ExportTests(unittest.TestCase):
    def test_chooser_keeps_exact_messages_and_index(self):
        row = example(choice="beat the gym leader here")
        reason, sample, identity = candidate(row)
        self.assertEqual(reason, "accepted")
        self.assertEqual(sample["prompt"][1]["content"], "unique prompt")
        self.assertEqual(json.loads(sample["completion"][0]["content"]),
                         {"choice": 2, "intent": "Prepare for the next badge."})
        self.assertEqual(identity["seed"], 3)

    def test_one_step_plan_but_not_unverified_plan_tail(self):
        row = example(task="strategic_plan", choice="heal your party")
        reason, sample, _ = candidate(row)
        self.assertEqual(reason, "accepted")
        self.assertEqual(json.loads(sample["completion"][0]["content"]),
                         {"goal": "earn the badge", "steps": ["heal your party"]})
        row["plan_steps"].append("beat the gym leader here")
        self.assertEqual(candidate(row)[0], "unverified_plan_tail")
        self.assertEqual(candidate(row, allow_multistep=True)[0], "accepted")

    def test_rejection_and_mismatch_never_become_supervision(self):
        base = example()
        variants = [
            ({"rejected": True}, "rejected"),
            ({"execution": None}, "unverified_execution"),
            ({"execution": {**base["execution"], "outcome": "blocked"}}, "unsuccessful_execution"),
            ({"execution": {**base["execution"], "objective": "some other action"}}, "unmatched_execution"),
            ({"execution": {**base["execution"], "round": 6}}, "unmatched_execution"),
            ({"system_prompt": ""}, "missing_exact_prompts"),
            ({"prompt_hash": ""}, "missing_provenance"),
            ({"offered": ["heal your party", "heal your party"]}, "invalid_choice"),
            ({"game": "pokemon-gold"}, "not_gen1_red"),
        ]
        for patch, expected in variants:
            with self.subTest(expected=expected):
                self.assertEqual(candidate({**base, **patch})[0], expected)

    def test_split_is_stable_per_seed_across_attempts(self):
        a = {"seed": 17, "game": "pokemon-red", "rom_sha256": "rom"}
        b = {**a, "run_id": "different-run", "attempt": 8}
        self.assertEqual(split_for(a, "fixed-salt", 10, 10),
                         split_for(b, "fixed-salt", 10, 10))

    def test_export_dedup_and_manifest(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            rows = [
                example(prompt="one"),
                example(prompt="one", seed=999, run="different"),  # identical model input
                example(prompt="plan", task="strategic_plan"),
                example(prompt="bad", choice="unknown"),
            ]
            corpus(root / "captures", rows)
            result = prepare([str(root / "captures")], root / "out",
                             validation_pct=0, test_pct=0)
            self.assertEqual(result["counts"]["train"], 2)
            self.assertEqual(result["rejected"]["duplicate_input"], 1)
            self.assertEqual(result["rejected"]["invalid_choice"], 1)
            lines = (root / "out" / "train.jsonl").read_text().splitlines()
            self.assertEqual(len(lines), 2)
            self.assertTrue(all(set(json.loads(line)) == {"prompt", "completion"} for line in lines))
            self.assertEqual(len((root / "out" / "validation.jsonl").read_text()), 0)
            self.assertEqual(json.loads((root / "out" / "manifest.json").read_text())["version"], 1)

    def test_conflicting_targets_for_identical_input_are_all_excluded(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            one = example(prompt="same")
            other = example(prompt="same", choice="beat the gym leader here")
            third = example(prompt="safe")
            corpus(root, [one, other, third])
            result = prepare([str(root)], root / "out", validation_pct=0, test_pct=0)
            self.assertEqual(result["counts"]["train"], 1)
            self.assertEqual(result["rejected"]["conflicting_duplicate"], 2)

    def test_corrupt_gzip_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "planner-training-v1.jsonl.gz").write_bytes(b"not-gzip")
            with self.assertRaises((OSError, EOFError, ValueError)):
                prepare([str(root)], root / "out", validation_pct=0, test_pct=0)
            self.assertFalse((root / "out" / "train.jsonl").exists())


if __name__ == "__main__":
    unittest.main()
