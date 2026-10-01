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

If `regressed` is true, this failure was observed on `observed_revision`,
which already contains the issue's resolution baseline: the earlier fix did
not hold for this occurrence. Read the closing PR/comment before patching.

If `mode` is `repair_pr`, this is an existing PR with failed checks. Stay on
`head_ref`, fix `failing_checks`, run `make test-short`, commit, and push.
Do not open another PR.

For a fresh failure:

1. Confirm the prepared reproduction is still a software defect. A live run,
   cancellation, expected gameplay, or infrastructure-only loss does not earn a
   gameplay patch.
   If current `main` already fixes it (a commit newer than the observed
   revision, and the replay on this tree reaches `objective_succeeded` with no
   changes), do not patch. When `issue_number` exists, comment on the issue
   with the fixing commit and the replay result, then
   `gh issue close <issue_number> --reason completed`. A closed issue is not
   picked again, and a later recurrence on a revision containing the fix
   reopens it. Without a replay, do not close.
   Whenever you do not ship a PR, write `.pokepilot-verdict.json` at the
   repository root: `{"verdict": "<v>", "reason": "<evidence>"}` where `<v>`
   is `already_fixed`, `cannot_reproduce`, `not_a_defect`, or `needs_human`.
   Name the commits, replay results, or missing artifacts in `reason`. A
   confirmed verdict stops further attempts until the failure occurs again;
   omit the file only when you simply ran out of ideas, so a stronger model
   tries next.
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
