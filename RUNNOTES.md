# RUNNOTES — S5c-5b: every path planner now plans from a live sprite snapshot

## What changed
- `skill/goto.go` (walkWithinMap), `skill/warp.go` (both Traverse cases):
  replaced the S5c-5a stubs with `func() map[[2]int]bool { return spriteBlockers(m) }`.
  That is the whole wiring — no planning-logic changes.
- `skill/story.go` (walkLab): deleted its own retry loop (local maxRetries=4,
  collision-ban map) and refactored onto walkAround. readBlocked is
  `mergeBlockers(spriteBlockers(m), blocked)` — the lab's static exclusions
  (rival (4,3), Oak (5,2), ball tiles (6..8,3)) are merged into every FRESH
  snapshot; mergeBlockers returns a new map, so no collision can ever mutate
  the fixed set. `labBlockedSet()` unchanged; callers unchanged.
- `skill/move.go`: the verbatim contract comment now sits above WalkPath
  ("Movement never advances dialogue... never answers a choice"). It is the
  contract half of slice 6's recovery layer; no code in move.go changed.

## Behaviour notes for S5c-6
- walkLab retry count is now walkAround's maxWalkRetries=6 (was 4), waits
  npcWaitFrames=48 between attempts.
- walkLab plan errors: the merged set is never empty, so walkAround retries
  up to 6x before returning the plan error AS-IS (GoTo planErr passthrough).
  Error wraps preserved: "no path on map 0x28...", "blocked ... after %d
  retries", "walk on map 0x28 ...". ErrDialogueInterrupted still passes
  through immediately (GetStarter step 8 relies on errors.Is).
- Pre-existing gofmt deviation in goto.go/warp.go (closure-body indent from
  S5c-5a) left alone to keep the diff minimal.

## Verified
- `POKEMON_RED_ROM=/home/maestro/Documents/projects/PokePilot/roms/pokemon_red.gb
  go test ./... -skip TestGymBoulderBadge`: all ok, 163 pass, 1 skip (TestProbe's
  permanent PROBE_MAP env gate). TestGetStarter (ROM-backed, both walkLab calls)
  PASS — the static ball-tile exclusions survived the refactor.
- TestGymBoulderBadge stays OUT of the gate: rDIV-seeded battles, S5c-6 owns the
  badge proof. Do not re-add it or "fix" it in wiring tasks.

## For the next task
- ROM: /home/maestro/Documents/projects/PokePilot/roms/pokemon_red.gb (main
  checkout). No roms/ in this worktree (gitignored) — use the env var, no symlink.
- Do not commit; leave edits uncommitted for the runner.
