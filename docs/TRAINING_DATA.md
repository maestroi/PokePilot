# Planner training corpus (v1)

PokePilot can collect **raw supervised fine-tuning candidates from its existing
single-model planner**. There is no second model, new gameplay policy, or change
to objective legality. Capturing is **off by default**.

## Enable capture

Opt into capture for a farm run by setting the per-run spec field:

```json
{
  "planner": "llm",
  "game": "pokemon-red",
  "training_capture": true
}
```

The operator API accepts the field as part of the run spec. The launch UI does
not yet expose a toggle. Existing runs without `training_capture` behave as
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
