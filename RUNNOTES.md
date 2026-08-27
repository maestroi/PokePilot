# RUNNOTES — S5c-5a: walkAround plans from fresh RAM instead of remembering guesses

## What changed
- `skill/blockers.go` (NEW): `spriteBlockers(m *emu.Emu) map[[2]int]bool` — snapshots RAM
  (`state.Snapshot`), runs `state.DecodeSprites`, keys `[2]int{X, Y}` (same order as
  walkAround/FindPath). `mergeBlockers(live, fixed)` returns the union as a new map.
  spriteBlockers is not called anywhere yet — S5c-5b wires it.
- `skill/move.go`: `walkAround(readBlocked, plan, walk, wait)` — readBlocked is called
  EXACTLY ONCE at the top of every attempt; that fresh map goes straight to plan. Plan
  error with live blockers: wait + retry (≤ maxWalkRetries). Plan error with none: return
  the static error unchanged. *ErrBlocked: wait + retry from a new snapshot. Other walk
  errors: return immediately. `hit` and `blocked` maps deleted (grep `hit` in move.go: gone).
  maxWalkRetries=6 / npcWaitFrames=48 untouched. Verbatim invariant comment + the two known
  races (IMAGEINDEX overlay is screen-local; TryWalking writes the destination tile at the
  start of the 16-frame animation, so a sprite can straddle two tiles) sit above walkAround.
- `skill/walkaround_test.go`: deleted the three twice-before-ban tests (WaitsOutAWanderingSprite,
  BansATileBlockedTwice, DropsItsOwnBansWhenPlanningFails). Added
  TestWalkAroundRereadsBlockersAfterCollision (read1 {(14,13)} → collision → read2 {(15,13)} →
  success; both snapshots reach plan in order) and TestWalkAroundForgetsVacatedSpriteTile
  (read1 {(14,13)} → collision → read2 empty; plan 2 gets NO (14,13) — the anti-cache proof).
  GivesUpAfterMaxRetries and DoesNotRetryOtherFailures kept byte-identical. Probe gained a
  `reads` list served by an injected readBlocked (past the end → empty map).

## For S5c-5b (wiring the callers)
- The walkAround signature change forced a mechanical compile fix in goto.go (walkWithinMap)
  and warp.go (both Traverse cases): a stub `func() map[[2]int]bool { return map[[2]int]bool{} }`
  first arg, marked `// S5c-5b wires this to spriteBlockers(m)`. Replace each stub with
  `func() map[[2]int]bool { return spriteBlockers(m) }` — that is the whole wiring; no
  planning-logic changes were made to those files.
- A nil/empty read is safe: FindPath documents `blocked may be nil`.

## Verified
- `go test ./... -skip TestGymBoulderBadge` (POKEMON_RED_ROM set to the main checkout's
  roms/pokemon_red.gb; this worktree has no roms/ — it is gitignored, and a symlink shows
  as untracked, so do not create one): 163 pass, 0 fail. Only skip is TestProbe's permanent
  PROBE_MAP env gate. TestGymBoulderBadge stays out of the gate (S5c-6, rDIV-seeded).
