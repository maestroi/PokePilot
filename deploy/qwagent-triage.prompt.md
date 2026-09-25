# Farm triage attempt

You are running unattended in a dedicated worktree. The shell already
chose the failure. Do not call pokepilot_get_triage to pick another one.

First load the repository triage instructions from
`.claude/skills/pokefarm-triage/SKILL.md`. If your agent runtime exposes a
native skill tool, you may load `pokefarm-triage` through that tool instead.
Then read and follow `docs/ARCHITECTURE.md`.

The packet JSON is attached. Use its `key`, `run_id`, and `example`. When
`issue_number` is present, it is the generated GitHub farm issue linked to this
failure group. For a fresh repair, the shell has already assigned that issue to
the authenticated GitHub user as the visible claim; do not remove or replace
that assignment. The shell has also applied the local PokéWall + GitHub
claim/repair/regression state machine; do not second-guess queue eligibility
from stale Orchestrator status.

If `mode` is `repair_pr`, this attempt is a pull request this loop already
opened and whose checks failed. Do not investigate a new farm failure.

1. The shell has checked out `head_ref`. Stay on that branch.
2. Fix the checks named in `failing_checks`. `make test-short` must pass.
3. Commit on `head_ref` and `git push`.
4. Do not open a new pull request and do not switch to `main`.

Otherwise do:

1. `pokepilot_get_run_debug` / `pokepilot_get_run_artifacts` for `run_id`.
2. Download the failing round `.state` (objective matching finish.detail).
3. Write `skill/zz_repro_scratch_test.go`, reproduce, then fix the shared
   invariant — not a named-map special case.
4. Re-run the repro. Delete the scratch test.
5. `make test-short`. Never `go test ./skill/...`.
6. If either gate is red, stop. Do not commit.
7. `git switch -c fix/<short-slug>` from this worktree's `main`.
8. Commit only the fix. Message: symptom, root cause, run id.
9. `git push -u origin HEAD`
10. `gh pr create` with title
    `fix(farm): <short symptom> [triage:<key>]`
    Body: run id, fingerprint, what you reproduced, what you changed. If the
    packet contains `issue_number`, also include `[farm-issue:<issue_number>]`
    in the body so the merge lifecycle closes that exact generated farm issue.

Do not:

- commit or push `main`
- commit `.gb`, `.sav`, `.state`, or `skill/zz_*_test.go`
- `git add -A` when unrelated files are dirty
- merge the PR
- start another farm run unless you need an artifact URL you cannot get
  from the existing run
