#!/usr/bin/env python3
"""Opt-in LoRA/QLoRA SFT for PokePilot's existing OpenAI-chat strategist.

Does not touch production deployments, launch a farm run, or upload weights.
"""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import sys


def count_rows(path: Path) -> int:
    with path.open(encoding="utf-8") as stream:
        return sum(bool(line.strip()) for line in stream)


def validate_dataset(directory: Path, minimum: int) -> tuple[Path, Path, dict]:
    manifest_path = directory / "manifest.json"
    if not manifest_path.is_file():
        raise ValueError(f"missing dataset manifest: {manifest_path}; run prepare.py first")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    if manifest.get("version") != 1 or manifest.get("format") != "trl-conversational-prompt-completion":
        raise ValueError("unexpected dataset version/format")
    train = directory / "train.jsonl"
    validation = directory / "validation.jsonl"
    if not train.is_file() or not validation.is_file():
        raise ValueError("missing train.jsonl or validation.jsonl")
    n_train, n_val = count_rows(train), count_rows(validation)
    if n_train < minimum:
        raise ValueError(f"only {n_train} training examples; at least {minimum} required")
    if n_val < 1:
        raise ValueError("no validation examples: collect more independent seeds or change split salt")
    if manifest.get("counts", {}).get("train") != n_train or manifest.get("counts", {}).get("validation") != n_val:
        raise ValueError("dataset manifest count mismatch")
    return train, validation, manifest


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dataset", type=Path, required=True, help="prepare.py output directory")
    parser.add_argument("--model", required=True, help="approved locally installed or HF base model name")
    parser.add_argument("--revision", default=None, help="pin model revision for reproducibility")
    parser.add_argument("--out", type=Path, required=True, help="local LoRA adapter output directory")
    parser.add_argument("--epochs", type=float, default=2.0)
    parser.add_argument("--learning-rate", type=float, default=1e-4)
    parser.add_argument("--rank", type=int, default=16)
    parser.add_argument("--batch-size", type=int, default=1)
    parser.add_argument("--gradient-accumulation", type=int, default=8)
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--max-length", type=int, default=None,
                        help="optional token limit; defaults to no truncation")
    parser.add_argument("--quantize-4bit", action="store_true",
                        help="NF4 QLoRA (requires CUDA and bitsandbytes)")
    parser.add_argument("--allow-cpu", action="store_true",
                        help="only for tiny local experiments; no 4-bit CPU fallback")
    parser.add_argument("--minimum-examples", type=int, default=50)
    args = parser.parse_args(argv)

    if (args.epochs <= 0 or args.learning_rate <= 0 or args.rank <= 0
            or args.batch_size <= 0 or args.gradient_accumulation <= 0
            or args.minimum_examples <= 0 or (args.max_length is not None and args.max_length <= 0)):
        parser.error("training arguments and minimum examples must be positive")
    try:
        train, validation, manifest = validate_dataset(args.dataset, args.minimum_examples)
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        parser.error(str(exc))
    if args.out.resolve() == args.dataset.resolve():
        parser.error("adapter output directory must differ from source dataset")

    # Heavy GPU dependencies are deliberately imported only after all dataset
    # validation. This keeps prepare.py and its unit tests dependency-free.
    try:
        import torch
        from datasets import load_dataset
        from peft import LoraConfig
        from transformers import AutoModelForCausalLM, AutoTokenizer, BitsAndBytesConfig
        from trl import SFTConfig, SFTTrainer
    except ImportError as exc:
        parser.error(f"install scripts/planner_training/requirements.txt first: {exc}")

    if not torch.cuda.is_available() and not args.allow_cpu:
        parser.error("no CUDA GPU detected; use a training host or explicitly pass --allow-cpu")
    if args.quantize_4bit and not torch.cuda.is_available():
        parser.error("4-bit QLoRA requires a supported CUDA GPU")
    if args.quantize_4bit:
        compute_dtype = torch.bfloat16 if torch.cuda.is_bf16_supported() else torch.float16
        quantization = BitsAndBytesConfig(
            load_in_4bit=True, bnb_4bit_quant_type="nf4",
            bnb_4bit_use_double_quant=True, bnb_4bit_compute_dtype=compute_dtype,
        )
    else:
        quantization = None
    tokenizer = AutoTokenizer.from_pretrained(args.model, revision=args.revision, trust_remote_code=False)
    model = AutoModelForCausalLM.from_pretrained(
        args.model, revision=args.revision, trust_remote_code=False,
        quantization_config=quantization, device_map="auto",
        torch_dtype="auto",
    )
    if tokenizer.pad_token is None:
        tokenizer.pad_token = tokenizer.eos_token

    # Hugging Face's conversational prompt-completion dataset masks prompts:
    # loss is computed on model answers, not on PokePilot's observation text.
    dataset = load_dataset("json", data_files={"train": str(train), "validation": str(validation)})
    bf16 = torch.cuda.is_available() and torch.cuda.is_bf16_supported()
    config = SFTConfig(
        output_dir=str(args.out),
        num_train_epochs=args.epochs,
        per_device_train_batch_size=args.batch_size,
        gradient_accumulation_steps=args.gradient_accumulation,
        learning_rate=args.learning_rate,
        gradient_checkpointing=True,
        logging_steps=10,
        save_strategy="epoch",
        eval_strategy="epoch",
        completion_only_loss=True,
        packing=False,
        max_length=args.max_length,
        bf16=bf16,
        fp16=torch.cuda.is_available() and not bf16,
        report_to="none",
        push_to_hub=False,
        seed=args.seed,
    )
    trainer = SFTTrainer(
        model=model, args=config, train_dataset=dataset["train"],
        eval_dataset=dataset["validation"], processing_class=tokenizer,
        peft_config=LoraConfig(
            r=args.rank, lora_alpha=args.rank * 2, lora_dropout=0.05,
            target_modules="all-linear", task_type="CAUSAL_LM",
        ),
    )
    trainer.train()
    trainer.save_model(str(args.out))
    tokenizer.save_pretrained(str(args.out))
    info = {
        "format_version": 1, "type": "pokepilot-planner-lora",
        "base_model": args.model, "base_revision": args.revision,
        "quantized_4bit": args.quantize_4bit, "seed": args.seed,
        "dataset_manifest_sha256": hashlib.sha256(
            (args.dataset / "manifest.json").read_bytes()
        ).hexdigest(),
        "dataset_counts": manifest["counts"],
        "tasks": manifest["tasks"],
        "note": "Offline adapter only; requires independent ROM-backed benchmark before deployment.",
    }
    (args.out / "pokepilot-training.json").write_text(
        json.dumps(info, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    print(f"Saved local LoRA adapter to {args.out}; no production deployment modified.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
