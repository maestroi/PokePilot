---
name: pokefarm-run-fix
description: Use when the user gives one PokePilot run id and wants it fixed. Start with the one-id compact debugger, use its deterministic replay and localized source context, edit the shared invariant, verify the same replay plus short tests, and open/record a triage-keyed PR.
---

# PokeFarm run fix

Input: **one run id**. Do not begin by reading the full run history, searching the
whole repository, or downloading artifacts by hand.

## 1. Prepare the bug in one command

Run:

```bash
make -s debug RUN=<run-id>
```

This command is the default evidence path. It:

- calls `pokepilot_prepare_debug`, which compresses run/finish/triage evidence;
- resolves the structured `failure-repro.json`, checkpoint, and paired knowledge;
- builds a local ROM-free repro bundle under the user cache directory;
- when `POKEMON_RED_ROM` is available, replays the exact objective through the
  current checkout without an LLM;
- searches the current checkout for the bounded `search_terms` and includes
  only nearby source snippets.

Use the returned `packet`, `reproduction`, and `source_matches`. Do not
re-fetch the same evidence with separate MCP calls.

Important reproduction states:

- `reproduced` — the current checkout reproduced the same structured failure;
- `fixed_or_not_reproduced` — the captured objective succeeds on this checkout;
- `different_failure` — the state is valid but behavior diverged; inspect that result;
- `skipped` / `not_available` — no supported deterministic contract; use the
  escalation path below only as needed.

The packet's `classification_hint` is deliberately conservative. A live run,
cancellation, or infrastructure-only loss does not earn a gameplay PR.

## 2. Claim only real repair work

If the packet names a triage issue/key and the failure still reproduces:

1. inspect the linked GitHub issue assignees;
2. stop if another agent owns it unless the user explicitly asked you to resume it;
3. otherwise claim the issue and call `pokepilot_investigate_failure(key)`.

If the deterministic replay already succeeds and the failure was observed on an
older revision, treat it as stale/fixed evidence unless newer evidence proves a
regression.

## 3. Edit the smallest owning invariant

Start with `source_matches`. Read outward only far enough to understand the
producer and immediate caller of the failure. Prefer a shared runtime/skill
invariant over a named map, NPC, route, or one-run exception.

Only broaden repository search if the prepared matches do not identify the
owner. For geometry use `world-map-debug`; for instruction/RAM questions use
`gomeboy-forensics`.

## 4. Verify

After the patch, rerun the exact same command:

```bash
make -s debug RUN=<run-id>
```

For a structured Red failure, the expected transition is:

```text
reproduction.state: reproduced
            -> fixed_or_not_reproduced
reproduction.classification: same_failure_reproduced
            -> objective_succeeded
```

Then run:

```bash
make test-short
```

Do not ship with either gate red.

## 5. Ship and record

Create a `fix/<slug>` branch from current `origin/main`, commit only the
repair, push, and open a PR. When a triage key exists, title it:

```text
fix(farm): <short symptom> [triage:<key>]
```

The PR body should name the run, root cause, deterministic replay transition,
and `make test-short` result. Never commit ROMs, saves, states, repro-cache
files, or scratch tests.

Record the solver attempt with `pokepilot_record_solver_attempt`. Reuse one
stable attempt id as its state advances.

## Escalation path

Use this only when `make debug` says the compact evidence is insufficient:

1. `pokepilot_get_run_debug(run_id)` for the full compacted timeline;
2. `pokepilot_get_run_recovery_audit(run_id)` when earlier recovery may have
   poisoned the failing checkpoint;
3. fetch a specific artifact only when one of those packets points to it.

Do not jump straight to raw recordings, full logs, or broad repository reads.
