---
name: pokefarm-run-diagnose
description: Use when the user gives one PokePilot run id and asks what went wrong. Use the one-id compact debugger first; it returns the failure identity, triage state, deterministic replay result when available, and localized source context. Diagnose only unless the user asks for a repair.
---

# PokeFarm run diagnosis

Input: **one run id**.

Start with exactly:

```bash
make -s debug RUN=<run-id>
```

That command is intentionally the cheap path: it replaces separate run-debug,
triage, artifact, reproduction, and repository-search steps with one bounded
JSON result.

Read:

- `packet.finish` and `packet.failure` for the terminal symptom/error chain;
- `packet.triage` for stable identity and existing issue state;
- `reproduction` for measured current-checkout behavior;
- `source_matches` for the likely producer/caller without broad code search.

Treat `classification_hint` as a hint, not a verdict. In particular,
`live_or_pending`, cancellation, expected gameplay, and infrastructure-only
runner loss are not software defects by themselves.

If `reproduction.state == reproduced`, the same structured objective failed
on the current checkout. If it is `fixed_or_not_reproduced`, compare the
captured runner revision with the current/fixed revision before calling it a
regression.

Only if the compact result is genuinely ambiguous, expand in this order:

```text
make -s debug RUN=<run-id> DEBUG_MODE=deep
pokepilot_get_run_debug(run_id)
pokepilot_get_run_recovery_audit(run_id)   # only for recovery-history questions
specific artifact content                  # only when named by the evidence
```

Do not download recordings or read large source areas for a normal diagnosis.

Report briefly:

```text
Run:        <id> @ <observed revision>
Class:      defect | expected gameplay | infrastructure | still live | ambiguous
Symptom:    <leaf error>
Repro:      <reproduced / succeeds now / unavailable>
Root cause: <why, file:line when localized>
Tracked:    <triage key / issue / status>
Next:       repair this run id | no code action | specific escalation
```

If the user wants a fix, hand the same run id to `pokefarm-run-fix`; do not
repeat discovery.
