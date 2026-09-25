---
name: pokefarm-run-diagnose
description: Use when the user gives a PokePilot run id (e.g. "run-9avakysua1ml", "what's wrong with run X", "why is this run stuck/failing") and wants a diagnosis. Reads the run's own evidence (finish detail, trace tail, failure frame, circuit/issue state), traces the error chain to the owning code, and reports the root cause. Diagnosis only — hand off to pokefarm-triage for repro + fix.
---

# PokeFarm run diagnosis

Input: one run id. Output: what went wrong, where in the code, and whether it
is already tracked. Most of the time the run's own evidence is enough — do not
download states or reproduce unless the evidence is genuinely ambiguous.

Do not edit code, file issues, or open PRs unless the user asks. If they ask
for a fix, hand off to `.claude/skills/pokefarm-triage/SKILL.md` (it owns
reproduce-before-fix).

## 1. Pull the debug bundle

```
pokepilot_get_run_debug(run_id=<run-id>)
```

This one call is usually the whole investigation. Read, in order:

- `finish.reason` / `finish.detail` — the failure class, e.g.
  `stabilization_failed objective_boundary_dirty progress=victory_road_cleared`.
- `finish.trace_tail` — the **last line is the full wrapped error chain**
  (`skill: A: skill: B: ... : <leaf error>`). The leaf is the actual symptom;
  the wrappers are the call path.
- `finish.runner_version` — the revision that failed.
- `run.issue` — the existing triage fingerprint/issue: `issue_number`,
  `status`, `circuit_count`, `circuit_open`, `verification_state`
  (`regressed` = came back after a fix), `quarantined_count`.
- `run.error_attempts` vs `run.attempts`, `run.circuit_count` — one-off
  failure or a retry loop hitting the same wall?
- `run.player` — party (lead slot, levels, HP), bag, badges. Often explains
  *why* the leaf happened (weak lead can't flee, no HM, no items).
- `progress_early` vs `progress_final` — did the run advance at all?
- `timeline` — what objective started right before the failure; repeated
  identical `started → failed → retry` rows mean a deterministic loop.

If the run is still live with no terminal reason, say so — look at
`pokepilot_get_run(run_id)` for current state, but there may be nothing wrong
yet.

## 2. Cheap extra evidence (only if step 1 is ambiguous)

Small inline artifacts from the bundle, fetched with
`pokepilot_get_run_artifact_content(run_id, name)` (base64 → decode):

- `failure-frame-*.json` — emulator snapshot at failure: `map`, `x`, `y`,
  `in_battle`, `controllable`, menu state.
- `objective-failures.json` — every objective failure in the attempt.
- `round-*.failure-repro.json` — the objective/skill call that failed.
- `final-frame.png` — what was on screen (Read tool renders it).

Don't fetch `.state`/`.ram` just to diagnose; that is triage's repro step.

## 3. Trace the chain into the code

Grep each distinctive fragment of the error chain, leaf first:

```bash
grep -rn "still in battle after\|boulder puzzle resolve battle" --include='*.go' skill agent | grep -v _test
```

Read the function that produced the leaf and its immediate caller. The root
cause is usually in the **caller's handling** of the leaf (a fallback that
doesn't exist, a sentinel not checked, a postcondition not enforced), not in
the leaf itself. Name the owning layer per `docs/ARCHITECTURE.md` — shared
skill/runtime invariant vs Red adapter fact.

For map/position questions use the `world-map-debug` skill rather than
guessing geometry.

## 4. Report

Keep it short:

```
Run:        <id> @ <runner_version short>  (attempt N, M error attempts)
Symptom:    <leaf error, one line>  at <MAP> (x,y)
Context:    <objective>, <relevant party/bag facts>
Root cause: <why, pointing at file:line>
Tracked:    #<issue> <status>/<verification_state>, circuit <count>
            — or "not tracked"
Confidence: high | medium (what would confirm it)
Next:       pokefarm-triage on key <key>  |  expected gameplay, no action  |  ...
```

Call out explicitly when it is **not** a software defect (normal blackout,
infrastructure/runner loss, strategy gap like undertraining) — see the
classes in `pokefarm-recovery-audit`.

## Worked example: run-9avakysua1ml

- detail: `stabilization_failed objective_boundary_dirty progress=victory_road_cleared`
- chain leaf: `skill: Flee: still in battle after 3 attempts: map c6 at (6,15)`
  wrapped by `boulder puzzle resolve battle` → `Travel: flee`.
- failure frame: `in_battle: true`, VICTORY_ROAD_3F.
- party: lead Beedrill L30, Mewtwo L68 in slot 2 — slow lead vs Victory Road
  wilds, so RUN keeps failing.
- code: `skill/boulder_puzzle.go` `resolveBoulderWalkInterruption` →
  `fleeThenFight` (`skill/travel.go`) only fights on `ErrTrainerBattle`; a
  wild battle where flee exhausts is returned as an error with the battle
  still open → dirty boundary → identical retry.
- tracked: #1742 open, `verification_state: regressed`, circuit open after
  1007 matches.
- verdict: deterministic controller defect (no fight fallback after failed
  wild flee), not gameplay. Next: triage key `3f682357aca0b024`.
