---
name: pokefarm-recovery-audit
description: Use when a user points at one PokePilot run and asks which recoveries matter. Audit the run's recovery history, collapse duplicate evidence, correlate existing triage/issues and repair revisions, and produce an actionable recovery-debt report without fixing or filing work unless explicitly asked.
---

# PokeFarm recovery audit

This skill is **run-centric**, not failure-centric.

Use it when the user says things like:

- "investigate the recoveries in run X";
- "which recoveries from this long run still need fixing?";
- "sort the recovery noise from real bugs";
- "audit this successful run for hidden reliability debt."

The goal is to turn a long run's recovery history into a small, trustworthy work
queue. A run that reached eight badges or the League can still contain useful
software-defect evidence; successful recovery is evidence, not proof that the
underlying behavior is acceptable.

Do **not** start fixing code during the audit unless the user explicitly asked
for fixes as well. Do not create issues merely because an event says
"recovery".

## 1. Start from the dedicated audit packet

Call:

```
pokepilot_get_run_recovery_audit(run_id=<run-id>)
```

This is preferred over `pokepilot_get_run_debug` for the first pass. The
generic debug MCP tool deliberately keeps only the newest timeline events.
The recovery-audit tool instead returns the wall's full **bounded persisted
recovery/failure activity history** for that run, plus related triage groups,
including resolved groups.

The packet contains:

- compact run and finish identity;
- recovery events with attempt and runner revision when recoverable from the
  attempt-start history;
- recovery counts/kinds;
- progress summary;
- triage groups that name this run;
- linked issue lifecycle, fixed revision, and actionable state when known.

Pokewall's activity history is itself bounded, so this is not a promise of an
infinite event log. Durable structured failure/triage evidence is the stronger
source for old defects that have rolled out of the activity ring.

Use `pokepilot_get_run_debug(run_id)` only for nearby context after the audit
packet has identified a specific event/group that needs it. Use artifacts only
when evidence is genuinely needed.

## 2. Classify, do not count raw events as bugs

Every meaningful recovery belongs in one of these classes:

### expected_gameplay

Normal game consequences or intentional strategy, not a software defect.

Examples:

- an ordinary blackout after losing a difficult battle;
- healing/restocking chosen proactively by normal strategy;
- retrying the Elite Four after a legitimate loss.

Repeated expected gameplay can still expose a **strategy/readiness gap**, but
it is not automatically a recovery-system bug.

### infrastructure

Runner loss, deployment rollover, worker restart, or other compute/control-plane
recovery. Keep this separate from gameplay quality.

### recovered_defect

A software/controller/navigation/planning defect occurred and recovery hid it
well enough for the campaign to continue.

This is actionable even if the run later made major progress.

### duplicate

The same underlying defect is already represented by another event/group or an
existing active issue. One defect gets one work item.

### stale_fixed

The event happened on a revision before a known repair and there is no proven
post-fix recurrence in the evidence being audited.

Do not reopen or recreate work from stale evidence.

### regression

The same fingerprint/cause occurred on a revision that **contains** the merged
repair that was supposed to fix it.

A regression is stronger evidence than stale issue metadata.

### strategy_gap

The software behaved as designed, but normal strategy repeatedly chose an
unproductive state: undertraining, poor resource preparation, repeated
unfavorable challenge attempts, etc.

Keep this separate from controller/runtime defects. It may deserve an
enhancement issue, but only after proving it is not expected variance.

### insufficient_evidence

There is not enough evidence to decide. Preserve the finding; do not turn
uncertainty into a new issue.

## 3. Collapse duplicates before investigating deeply

Use this identity order:

1. existing triage `key` / `fingerprint`;
2. structured failure identity/family when present;
3. otherwise semantic recovery cause + subsystem + runner revision;
4. only as a last resort, normalized summary/detail wording.

Do not treat coordinates, frame numbers, attempt numbers, or run IDs as defect
identity.

For each collapsed group retain:

- count;
- first/last attempt;
- first/last frame when available;
- runner revision(s);
- recovery kind(s);
- linked issue;
- fixed revision/resolution;
- representative evidence.

A hundred identical retries should become one finding with count=100, not a
hundred investigations.

## 4. Handle long-running revisions correctly

Long campaigns can cross deployments. The revision that produced a recovery is
part of the evidence.

When a linked issue has a `fixed_revision`:

1. identify the recovery event's `runner_version`;
2. prove Git ancestry before calling it stale or a regression;
3. if the repair is an ancestor of the observed runner revision and the same
   defect reproduced, classify **regression**;
4. if the event predates the repair, classify **stale_fixed** unless later
   evidence shows recurrence;
5. if ancestry cannot be proven, use **insufficient_evidence** rather than
   guessing from timestamps.

With a local checkout, the proof is conceptually:

```sh
git merge-base --is-ancestor <fixed-revision> <observed-runner-revision>
```

Equivalent repository ancestry evidence is fine. A later attempt of the same
run does not retroactively change the revision that produced an earlier event.

## 5. Distinguish recovery from normal strategy

Do not penalize healthy behavior just because it restores resources.

These are normally **not abnormal recovery**:

- choosing a Pokémon Center before a major challenge;
- buying medicine before the League;
- deliberate training because readiness is low;
- normal Fly/Teleport/Dig travel selected as strategy;
- switching party members for XP or battle tactics.

These **are** recovery/reliability evidence when caused by a failure state:

- breaking a navigation loop;
- escaping a controller/menu dead-end;
- rolling back/checkpoint-resuming after a software blocker;
- retrying a failed objective because execution malfunctioned;
- circuit-breaker intervention;
- restoring from an unexpected runtime failure.

The distinction is causality, not the move/item used.

## 6. Produce a recovery-audit report

Report the collapsed findings, not a chronological dump.

Use a compact table with:

| finding | count | attempts/revisions | classification | existing issue | action |
|---|---:|---|---|---|---|

Then summarize totals such as:

- raw recovery events observed;
- collapsed findings;
- expected gameplay;
- infrastructure;
- duplicates/stale fixed;
- actionable recovered defects;
- regressions;
- strategy gaps;
- insufficient evidence.

For actionable findings, explain the concrete symptom and why it is not merely
expected gameplay.

Prioritize:

1. regressions;
2. current-revision recovered defects;
3. repeated/high-cost strategy gaps;
4. older unresolved recovered defects;
5. insufficient evidence.

Do not assign a numeric severity solely from event count.

## 7. Issue behavior

During an audit:

- never file a duplicate issue when a triage fingerprint already owns the
  defect;
- never recreate an issue from a resolved pre-fix occurrence;
- refine an existing issue when new evidence materially improves its scope;
- create a new issue only when the user asked for issue creation and the
  finding is genuinely distinct/actionable;
- keep expected gameplay and infrastructure recoveries out of the gameplay bug
  queue.

If the user asks to **fix** the actionable results, hand them to
`.claude/skills/pokefarm-triage/SKILL.md` one stable failure at a time. That
skill owns exact-state reproduction and repair. Do not bypass its
reproduce-before-fix rule just because this audit already identified the likely
cause.

## 8. Recovery quality signal

For successful qualification/speedrun analysis, the useful long-term metric is
not the supervisor's current recovery depth. Track the audit result conceptually
as:

```text
abnormal recovery interventions per successful run
```

Keep expected gameplay and infrastructure separate. The desired trend is toward
zero software-defect interventions while retaining strong recovery machinery as
a safety net.
