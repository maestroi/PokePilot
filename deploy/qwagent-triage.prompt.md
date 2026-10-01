# Farm triage attempt

You are unattended in a dedicated worktree. The shell already chose exactly
one failure. Do not call `pokepilot_get_triage` to pick another one.

Read `.claude/skills/pokefarm-triage/SKILL.md` and follow it. The attached
Packet owns `key`, `run_id`, issue/claim state, and queue eligibility. Do not
re-evaluate queue selection from stale remote metadata.

When a **Prepared debug packet** is attached, start from it. It already contains
the bounded farm evidence, deterministic replay result when supported, and
localized source snippets. Do not repeat `get_run_debug`, artifact downloads,
or broad repository searches unless that packet explicitly leaves an ambiguity.

If `regressed` is true, the linked issue was closed by a merged
`[triage:<key>]` PR, but this failure was observed again on
`observed_revision`, which already contains that merge. The earlier fix did not
hold. "Already fixed" is not a valid outcome: find the merged PR, work out why
its invariant did not cover this occurrence, and fix it.

If `mode` is `repair_pr`, this is an existing PR with failed checks. Stay on
`head_ref`, fix `failing_checks`, run `make test-short`, commit, and push.
Do not open another PR.

For a fresh failure:

1. Confirm the prepared reproduction is still a software defect. A live run,
   cancellation, expected gameplay, or infrastructure-only loss does not earn a
   gameplay patch.
2. Start from the packet's `source_matches`; read only the producer/caller
   needed to understand the owning invariant.
3. Fix the shared invariant, not a named-map/NPC/run special case.
4. Rerun `make -s debug RUN=<run_id>`; a structured Red repro should move
   from the same failure to `objective_succeeded`.
5. Run `make test-short`. If either gate is red, do not ship.
6. Create/push `fix/<short-slug>` and open
   `fix(farm): <short symptom> [triage:<key>]`.
7. The PR body names the run, fingerprint, root cause, replay before/after, and
   test result. If `issue_number` exists, include
   `[farm-issue:<issue_number>]`.

Do not commit/push `main`, ROM/save/state/cache files, or scratch tests. Do
not `git add -A` over unrelated work. Do not merge the PR.
