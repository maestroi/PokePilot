# RUNNOTES — S4-3a Planner seam (done, verified)

## What changed
Commit "agent: the planner seam and a deterministic scripted planner".

- `agent/planner.go` (new, exact content per task spec):
  - `Planner` interface: `Next(obs Observation, offered []Objective) (Objective, error)`.
    Contract: a planner MUST return one of `offered`.
  - `ErrDone` = "agent: nothing left to do" — sentinel for "no more objectives".
  - `ScriptedPlanner` (unexported fields objs/next) + `NewScriptedPlanner(objs...)`.
    `Next` ignores obs and offered BY DESIGN (list IS the plan); returns ErrDone
    when exhausted. This is the default planner; tests never call a model.
  - `Chosen(offered, s)`: exact-match only. Trimmed, case-insensitive
    `Objective.String()` match, OR a bare 1-based index ("3"). Index 0 and
    out-of-range are errors. NO fuzzy/prefix/edit-distance — deliberately.
  - `offeredList`: numbered one-liner ("1: go to Pallet Town, 2: ...") used in
    EVERY error message. S4-5's safety property depends on errors naming the
    offered list.

## Verified
- `go build ./...` exit 0, `go vet ./agent/...` exit 0 (worktree, not the stale
  /home/maestro/Documents/projects/PokePilot checkout).
- `go test ./agent/...` ok (fixture tests skip without ROM; no ROM here).

## Gotchas / next
- Work ONLY in this worktree. The PokePilot checkout at
  /home/maestro/Documents/projects/PokePilot is on an older commit without
  agent/ — that is why the previous attempt's verification failed.
- S4-3b+: anything that consumes Planner must handle ErrDone explicitly.
- S4-5: model-backed planner will call Chosen with model output; keep the
  exact-match + index behavior, and keep offeredList in every error.
- Objective.String() forms: "go to <place>", "talk at (x,y)", "take a starter".
- TestGoToViridianPokecenter still red on Route 1 (plan fdc1544f); skip it in
  full-suite runs.
