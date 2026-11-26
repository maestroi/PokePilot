"""Dependency-free evaluation and training-setup guards."""
from __future__ import annotations

import json
from pathlib import Path
import tempfile
import unittest

from evaluate import offered_labels, parse_response
from train_lora import validate_dataset


class EvaluationTests(unittest.TestCase):
    def test_menu_parser_ignores_notes(self):
        prompt = (
            'Observation:\n{"round":2}\n\nOffered objectives:\n'
            '1: heal your party  (failed 2x)\n'
            '2: beat the gym leader here\n'
        )
        self.assertEqual(offered_labels(prompt), [
            "heal your party", "beat the gym leader here",
        ])

    def test_choice_checks_type_and_bounds(self):
        menu = ["heal your party", "beat the gym leader here"]
        self.assertEqual(parse_response("objective_choice", '{"choice":2}', menu),
                         ("valid", {"choice": 2}))
        for text in ('{"choice":true}', '{"choice":3}', '{"choice":0}', 'no'):
            self.assertNotEqual(parse_response("objective_choice", text, menu)[0], "valid")

    def test_plan_checks_legal_objective_sentences(self):
        menu = ["heal your party", "beat the gym leader here"]
        self.assertEqual(parse_response("strategic_plan",
                         '{"goal":"prepare","steps":["heal your party"]}', menu)[0], "valid")
        self.assertEqual(parse_response("strategic_plan",
                         '{"goal":"prepare","steps":["invented action"]}', menu)[0], "illegal_plan")
        self.assertEqual(parse_response("strategic_plan",
                         '{"goal":"prepare","steps":[]}', menu)[0], "illegal_plan")


class TrainingSetupTests(unittest.TestCase):
    def test_training_requires_nonempty_holdout_and_count_integrity(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            (root / "manifest.json").write_text(json.dumps({
                "version": 1, "format": "trl-conversational-prompt-completion",
                "counts": {"train": 2, "validation": 1},
            }))
            (root / "train.jsonl").write_text('{"prompt":[],"completion":[]}\n' * 2)
            (root / "validation.jsonl").write_text('{"prompt":[],"completion":[]}\n')
            train, validation, _ = validate_dataset(root, minimum=2)
            self.assertEqual(train.name, "train.jsonl")
            self.assertEqual(validation.name, "validation.jsonl")
            with self.assertRaisesRegex(ValueError, "at least 3 required"):
                validate_dataset(root, minimum=3)
            (root / "validation.jsonl").write_text("")
            with self.assertRaisesRegex(ValueError, "no validation examples"):
                validate_dataset(root, minimum=2)


if __name__ == "__main__":
    unittest.main()
