---
name: pokefarm-triage
description: Use for the actionable PokeFarm failure queue. Select or consume one stable failure, then use the compact one-id debugger to reproduce and localize it before editing. Fix the shared invariant, rerun the same deterministic replay, run short tests, and ship a triage-keyed PR.
---

# PokeFarm triage

The expensive evidence-gathering path is no longer the default. Once a failure
has a representative run id, use the one-id debugger rather than manually
calling multiple MCP tools or constructing scratch tests.

## Select one work item

Interactive queue work starts with:

```text
pokepilot_get_triage()
```

Use the stable triage group and representative `run_id`. Resolved groups are
history, not new work.

For unattended qwagent work, the shell has already selected the group. **Do not
pick another one.** Its attached packet contains `key`, `run_id`, issue
metadata, and normally a prepared debug result.

Claim the generated GitHub issue before editing when one exists. If another
agent already owns it, stop rather than duplicate work.

## Prepare/reproduce

If the unattended packet already contains a current **Prepared debug packet**,
use it directly. Otherwise run:

```bash
make -s debug RUN=<run-id>
```

The result resolves the exact structured failure contract, downloads only the
small state/knowledge/repro artifacts needed, runs deterministic executor replay
when supported, and localizes likely source lines.

A repair should normally start from `reproduction.state == reproduced`.
Infrastructure-only loss, expected gameplay, cancellation, and unfinished live
runs do not become code bugs merely because they appear in the queue.

## Fix

Start from `source_matches`; inspect the immediate producer/caller and fix the
shared invariant at its owning layer. Do not add a named-map/NPC special case
unless the game rule itself is genuinely map-specific.

Use specialized skills only when the packet demands them:

- `world-map-debug` for measured geometry/routing;
- `gomeboy-forensics` for instruction-level RAM/CPU questions;
- `pokefarm-recovery-audit` when an earlier recovery poisoned the checkpoint.

## Verify and ship

Rerun:

```bash
make -s debug RUN=<run-id>
make test-short
```

For a structured Red failure, the deterministic replay should move from the
same captured failure to `objective_succeeded`.

Push a `fix/<slug>` branch and open a PR titled:

```text
fix(farm): <short symptom> [triage:<key>]
```

Include the run id, root cause, replay before/after, and short-test result.
Never commit ROMs, saves, states, cached repro bundles, or scratch tests.
Record the solver attempt through `pokepilot_record_solver_attempt`.

## Escalate only when necessary

If the one-id packet cannot establish the cause, expand progressively:
`pokepilot_get_run_debug`, then recovery audit if relevant, then one named
artifact. Raw recordings and broad source searches are last-resort evidence,
not the starting context.
