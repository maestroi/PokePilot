# Planner training corpus (v1)

PokePilot can collect **raw supervised fine-tuning candidates from its existing
single-model planner**. There is no second model, new gameplay policy, or change
to objective legality. Capturing is **off by default**.

## Enable capture

Enable **Collect planner training data** in the operator's New Run form, or
opt in programmatically by setting the per-run spec field:

```json
{
  "planner": "llm",
  "game": "pokemon-red",
  "training_capture": true
}
```

The operator API and launch UI both support the setting. Existing runs without `training_capture` behave as
before. This is a diagnostic/data-collection feature, **not** a training
invocation.

At the end of an attempt, the runner writes `planner-training-v1.jsonl.gz`.
It is listed alongside the run's finish artifacts in Run Inspector. When
`POKEPILOT_S3_*` is configured it is stored in the existing S3-compatible
artifact store, under the same per-run/attempt prefix as `run.gbrun`.
With no S3 configuration it is attached inline if its compressed bytes are
at most 4 MiB and fit the overall finish limit. Collection and upload are
best-effort and cannot turn a successful gameplay run into a failed one.

## Data contract

Every JSONL row is one actual strategist or single-objective LLM **call**,
including rejected responses and retries, and includes:

- `version: 1` and `task: strategic_plan | objective_choice`.
- Run/attempt/seed/game/ROM SHA-256/build, plus per-call actual model,
  primary/fallback route, and prompt hash.
- Full semantic observation and complete objective menu given to the model.
- The plan goal, ordered plan steps, boundary and replan reason, or the
  single-objective choice; rejected calls retain an error without a label.
- Optional `execution` with exact round, executed objective, typed outcome,
  recovered/terminal flags, and before/after badge counts.

**An `execution` field is present only when the executed objective matches the
selected single objective or the FIRST planned step in the exact decision
round.** Cached later plan steps and later recovery cannot safely be credited
to that one call; the exporter does not guess. The absence of `execution`
means **unknown**, not failure.

These are candidates for subsequent filtering, not examples already labeled
as optimal. In particular, `completed` proves an objective's postcondition,
not that the decision improved the overall strategy.

## Read the corpus

```python
import gzip
import json

with gzip.open("planner-training-v1.jsonl.gz", "rt", encoding="utf-8") as f:
    for line in f:
        example = json.loads(line)
        if example.get("rejected", False):
            continue
        print(example["task"], example["round"], example.get("execution"))
```

Partition training/evaluation by **run and seed**, not individual row.
Deduplicate equivalent observations, downweight repeated stalls, and filter
direct-execution failures and unanswered choices. Preserve the prompt/schema
version when generating chat-format examples to avoid teaching one prompt's
answers with a different input format.

## Current limitations / next slices

- Capture is written at **attempt end**. A worker that dies without settling
  can lose the in-memory examples. Periodic crash-safe S3 segments are next.
- Outcome linkage is immediate only; there is no counterfactual reward for
  alternative actions or multi-round strategic plan quality yet.
- The existing in-memory `benchmarkCalls` is the source of all calls, avoiding
  the 64 strategic dashboard record cap. A long-running campaign still needs
  streaming capture to avoid this in-memory buffer growing indefinitely.
- This is **planner** data, not the separate sampled typed-decision training
  log proposed in [#1826](https://github.com/maestroi/PokePilot/issues/1826).
- Next: add dataset export/validation CLI, held-out evaluations on matched
  seeds, then train a small model and register it as an ordinary strategist
  deployment. Do not promote it based only on imitating the current model.

## Prepare a Gen 1 fine-tuning dataset (this slice)

The training toolchain lives in `scripts/planner_training/` and **does not
change PokePilot's single-LLM runtime**. Runs created before the exact-prompt
capture changes will not have `system_prompt` and `user_prompt` in their
corpus; the exporter rejects those rows instead of reconstructing an
inaccurate prompt. Launch new opted-in Red runs after this change is merged.

1. In **New Run**, choose Pokémon Red and enable **Collect planner training
   data**. Complete multiple diverse runs (different seeds and goals).
2. Download each run's `planner-training-v1.jsonl.gz` from its artifacts.
   Keep files under separate directories (for example
   `artifacts/planner-training/run-<id>/planner-training-v1.jsonl.gz`).
   These archives may contain full private gameplay observations and must
   **not** be committed to git.
3. Generate train/validation/test splits (Python 3.11+, no ML packages needed):

```sh
python3 scripts/planner_training/prepare.py \
  artifacts/planner-training \
  --out artifacts/planner-dataset \
  --task all

python3 -m unittest discover -s scripts/planner_training -p 'test_*.py' -v
```

The exporter strictly filters to matching successful, unrecovered,
nonterminal objective execution and requires actual prompt text, a verified
decision/plan, provenance, and a legal offer. For now, multi-step plans are
excluded: their full tail has not been independently verified. It deduplicates
identical model inputs globally, drops conflicting responses, and assigns
each **game + ROM + seed** to a stable split across runs and attempts.
The output is `train.jsonl`, `validation.jsonl`, `test.jsonl`, and a
`manifest.json` explaining filtering and sample counts. The split is
deterministic, not balanced. A few seed values can leave validation or test
empty; collect more runs rather than using training rows for evaluation.

Train and test examples use TRL's conversational prompt/completion format:
`system` and `user` are the *actual* requests sent to the planner, and the
assistant completion is the existing JSON contract, not an invented free-text
strategy. The "choice" output is the 1-based index into that exact menu.

## Train a small text-only LLM (LoRA / QLoRA)

The first tested-target design is an ordinary **causal text model** such as
`Qwen/Qwen3-1.7B`. Qwen3.5-2B uses a *multimodal* Transformers architecture,
so it is not assumed to work with this `AutoModelForCausalLM` entry point;
supporting that architecture is a separate training-path validation task.

Use a private training host and isolated Python environment:

```sh
python3 -m venv .venv-training
. .venv-training/bin/activate
pip install -r scripts/planner_training/requirements.txt
# Install a CUDA-compatible bitsandbytes build separately for --quantize-4bit.

python3 scripts/planner_training/train_lora.py \
  --dataset artifacts/planner-dataset \
  --model Qwen/Qwen3-1.7B \
  --out artifacts/planner-lora-v1 \
  --quantize-4bit
```

The trainer refuses a missing manifest, fewer than 50 training records, an
empty validation split, and a manifest count mismatch. It uses completion-only
loss, LoRA on linear layers, and **no silent prompt truncation** by default.
A local adapter and `pokepilot-training.json` are saved; nothing is uploaded
or registered automatically. An explicit model revision can be pinned with
`--revision`. Quantized QLoRA requires CUDA (CPU use is opt-in, non-quantized,
and suitable only for small tests). Review VRAM capacity and long-prompt sizes
before a production training run.

Before using traces from a third-party model, confirm that its license and
provider terms permit model training/distillation. Collected data is not
automatically proof that the teacher's strategy was optimal.

## Evaluate without modifying production

After serving the **base model** and the **candidate adapter** independently
through OpenAI-compatible endpoints (or sequentially on one inference GPU),
compare both on the same withheld set:

```sh
python3 scripts/planner_training/evaluate.py \
  --dataset artifacts/planner-dataset \
  --endpoint http://127.0.0.1:8000/v1 \
  --model pokepilot-candidate \
  --out artifacts/candidate-evaluation.json
```

`POKEPILOT_TRAIN_EVAL_TOKEN` is an optional API key supplied through the
environment, never the command line. Do not point this command at a
third-party server unless its use of full game observations is intended.

This reports valid schema responses, choices/steps within the offered menu,
latency and exact teacher agreement. **Teacher agreement does not measure
game success.** For promotion, register the adapter as a *candidate deployment*
and use the existing matched-seed PokePilot farm experiments /
`pokebench compare` to measure completed milestones, stalls, recovery loops,
time and cost. Keep the existing model default until those checks pass.

### Still to implement

- Crash-resilient streaming of training segments during endless attempts,
  rather than holding model calls in memory until Finish.
- Long-horizon credit assignment and a more sophisticated failure/quality
  labeler. A completed objective may still be an unproductive detour.
- A dedicated validated multimodal Qwen3.5 training path, if preferred, and
  automated offline-adapter serving/registration.
- GPU-backed training and a real paired qualification result on collected data.
