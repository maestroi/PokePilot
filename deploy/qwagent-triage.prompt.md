# Farm triage attempt

You are running unattended in a dedicated worktree. The shell already
chose the failure. Do not call pokepilot_get_triage to pick another one.

Read and follow `.claude/skills/pokefarm-triage/SKILL.md` and
`docs/ARCHITECTURE.md`.

The packet JSON is attached. Use its `key`, `run_id`, and `example`.

Do:

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
    Body: run id, fingerprint, what you reproduced, what you changed.

Do not:

- commit or push `main`
- commit `.gb`, `.sav`, `.state`, or `skill/zz_*_test.go`
- `git add -A` when unrelated files are dirty
- merge the PR
- start another farm run unless you need an artifact URL you cannot get
  from the existing run
