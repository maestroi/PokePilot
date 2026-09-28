---
name: pokefarm-run-fix
description: Use when the user hands over one PokePilot run id and asks for it to be fixed, unblocked, or made to continue ("run-9avakysua1ml is stuck", "why did run X stop — open a PR"). Resolves the run to its stable triage failure key, separates a software defect from a stall, expected gameplay, or infrastructure loss, and for a defect drives the pokefarm-triage reproduce-fix-verify loop through to a triage-keyed PR and a recorded solver attempt.
---

# PokeFarm run fix

Input: one run id. Output: either a PR that repairs the failure, or a short
verdict explaining why no PR is the right answer.

`pokefarm-run-diagnose` stops at the report. This skill is that same evidence
plus the fix and the PR, entered from a run id instead of from a queue packet.
From section 4 onward the procedure is `.claude/skills/pokefarm-triage/SKILL.md`
— read that file before editing code; it owns reproduce-before-fix, the
shared-invariant rule, and the verification gates.

Do not cancel, restart, or re-seed the run. The wall owns recovery and will
re-attempt on its own; the repair is what makes the next attempt different.
Stopping a run is `pokepilot_cancel_run` and a separate instruction.

## 1. Read the run, then classify it

```
pokepilot_get_run_debug(run_id)
```

The run is in exactly one of four shapes. Decide which before touching code;
three of them must not produce a PR.

| Shape | Evidence | Action |
|---|---|---|
| **Defect** | terminal `finish.reason`/`detail` with a controller error chain in `finish.trace_tail`, repeated identical `started → failed → retry` rows | fix it (section 4) |
| **Stall** | `run.error_attempts` climbing toward `run.attempts`, circuit opening, attempts ending `lost: no heartbeat` | a stall whose chain names a failure is a defect; one with no skill error is infrastructure |
| **Expected gameplay** | blackout, lost battle, under-levelled lead, a door or gate the story has not opened yet | report, no PR |
| **Infrastructure** | `lost: no heartbeat`, `runner drained for deployment`, runner-revision churn, no skill error anywhere in the chain | report, no PR |

For a run that is still live, `pokepilot_get_run(run_id)` has the current state
and `pokepilot_get_run_recovery_audit(run_id)` keeps the bounded recovery and
failure history that the normal debug timeline compacts away. That audit is the
answer to "is it stuck right now". A live run with no terminal reason is not a
repair item yet — say so instead of inventing one.

The last line of `finish.trace_tail` is the full wrapped error chain. Read it
from the leaf outward: the leaf names the symptom, the wrappers name the call
path, and the defect usually lives in a caller's handling of the leaf.

## 2. Resolve the run to its triage key

The debug bundle carries the fingerprint twice:

- `run.issue.fingerprint` — the full stable fingerprint (`sha256:...`);
- `run.issue.circuit_key` — the 16-hex triage **key** the rest of the farm uses
  (`[triage:<key>]`, `/v1/triage/<key>/...`). The same value appears in
  `run.stop_so_far` and in the `circuit` timeline rows.

Confirm it against the queue rather than assuming:

```
pokepilot_get_triage()                       # actionable groups only
pokepilot_get_triage(include_resolved=true)  # history: stale evidence vs regression
```

An absent group does not mean "no work": it can mean the fingerprint has no
linked issue yet, or that the linked issue is already resolved. An empty
`run.issue` means this run never tripped the wall's fingerprinting — the fix is
still worth making, but the PR cannot carry a `[triage:<key>]` claim marker and
there is nothing to record an attempt against.

When the run id is not in the actionable queue but its fingerprint has history,
`pokepilot_get_run_recovery_audit(run_id)` returns the related groups including
resolved ones. That is how stale pre-fix evidence is separated from a real
regression: a failure observed at a revision that already contains the merged
repair is actionable, older evidence is not.

## 3. Claim the group before editing code

The same ownership rule as `pokefarm-triage`:

1. read the linked issue's assignees (`gh issue view <number>`); if another
   agent already has it, stop and report that rather than duplicating work;
2. otherwise claim it (`gh issue edit <number> --add-assignee @me`);
3. mark the group investigating with `pokepilot_investigate_failure(key)` —
   the wall requires a linked issue there and answers `unknown failure group`
   when the key has none;
4. release the assignment if the attempt is abandoned before a PR exists.

## 4. Reproduce, then fix

Follow `pokefarm-triage` sections 3–5 exactly:

- pull the failing round's `.state` with
  `pokepilot_get_run_artifact_content(run_id, name)` and decode the base64 —
  never `curl` the Cloudflare-Access-fronted artifact route;
- replay that state through the skill the objective called in a throwaway
  `skill/zz_repro_scratch_test.go`: red before the patch, green after it;
- fix the shared invariant at its owning layer per `docs/ARCHITECTURE.md`, not
  the named map, NPC, or route that exposed it;
- delete the scratch test, then run `make test-short`. Never gate on bare
  `go test ./skill/...`.

Use `world-map-debug` for geometry, `gomeboy-forensics` for instruction-level
RAM questions, and `pokefarm-recovery-audit` when the failing round arrived
poisoned by an earlier attempt.

## 5. Ship the repair

Branch `fix/<short-slug>` from a freshly fetched `origin/main`, commit only the
repair, push, and open a PR whose title carries the claim marker:

```
fix(farm): <short symptom> [triage:<key>]
```

The body names the run id, the fingerprint, the reproduction, the root cause,
the fix, and the honest verification result. Never commit `.gb`, `.sav`, or
`.state` files, and never `skill/zz_*_test.go`. Do not merge — the repository's
own merge and deploy machinery owns that.

## 6. Close the loop

Record the attempt against the group so the farm can see who is on it and what
came out:

```
pokepilot_record_solver_attempt(
  key=<key>, id="<stable-attempt-id>", backend="dsh", model="<model>",
  state="pr_opened", run_id=<run-id>, branch="fix/<slug>",
  pr_number=<n>, pr_url="<url>", note="<one line>")
```

`id` must be stable across updates of the same attempt: reuse it to move
`started → no_pr → pr_opened → pr_updated`, and record `agent_failed` with the
exit code when the harness dies instead. The wall truncates fields at 256 bytes
and the note at 1024, keeps the newest 32 attempts, and rejects a key with no
linked issue. Issue resolution and PokePilot's own verification — not this
record — decide whether the repair actually worked.

## 7. Report

```
Run:        <id> @ <runner_version short>  (attempt N, M error attempts)
Class:      defect | stall | expected gameplay | infrastructure
Symptom:    <leaf error, one line>  at <MAP> (x,y)
Key:        <key>  issue #<n> <status>  — or "untracked"
Root cause: <why, file:line>
Fix:        PR <url> (<branch>)  |  no PR: <one-line reason>
Gates:      repro <red→green>; make test-short <pass|fail>
Recorded:   solver attempt <id> (<state>)
```

## Worked example: run-1wsyy1f75ssxsheu3o4xpui4

Everything below is `pokepilot_get_run_debug` output; no code has been read yet.

- `finish.reason = error`, attempt 171 of 171, `error_attempts = 69`;
- leaf: `Travel: still interrupted by a text box after 30 recoveries` →
  `GoTo` → `Traverse: walk to warp on map 01: text box interrupted movement`,
  at `VIRIDIAN_CITY (32,10)`, objective `go to viridian gym, fleeing wild battles`;
- the trace immediately before it is the gym-door dialogue (`The GYM's doors
  are locked`) plus `control: control lost (joyIgnore 0xff)` / `regained`, so
  the text box is that door script and the failed warp is the gym entrance;
- `run.issue.circuit_key = 6261b5adf8bad42b`, issue #2140 `open`, circuit open
  after 3 matching failures, `last_disposition = quarantine`;
- repeated `lost: no heartbeat` recoveries between the failures, so part of the
  171 attempts is infrastructure rather than the same defect;
- `progress_final` still shows 0 badges and the same 15-map coverage as
  `progress_early`: the retry loop was not making story progress.

Two candidate classes survive that evidence, and only the reproduction
separates them: expected gameplay (the door is scripted shut and the planner
should not have been sent there) versus a controller defect (30 recovery
attempts that never dismiss a text box it is standing inside). That fork is
exactly what section 1 exists to force — do not settle it from the error string.
