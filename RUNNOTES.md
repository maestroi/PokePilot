# RUNNOTES — FIX-3 verified movement ground truth (DONE)

## What I changed
- `skill/move_test.go` only (move.go untouched, as required). Rewrote the
  four existing tests with values measured against the real ROM and added
  `TestStepOnceEveryOpenDirection`:
  1. `TestStepOnceMoves`: start asserted (3,6); StepLeft -> (2,6), no err.
  2. `TestStepOnceIntoWallIsBlocked`: StepUp -> *ErrBlocked via errors.As,
     coords still exactly (3,6) (asserts the move did NOT happen — this is
     what catches a "everything is blocked" StepOnce).
  3. `TestWalkPathTwoSteps`: [Left, Left] -> exactly (1,6).
  4. `TestWalkPathIsDeterministic`: two fresh fixture loads, same path,
     both end at exactly (1,6) (not just equal to each other).
  5. `TestStepOnceEveryOpenDirection`: DOWN -> (3,7), LEFT -> (2,6),
     RIGHT -> (4,6), each on a freshly loaded fixture.
- Removed the old guessed collision-grid comment; replaced with a note
  that expectations are ROM-measured ground truth.

## Why
The original PP-13 "walk DOWN twice" expectation was a guess and wrong:
y=7 is the bottom walkable row, so the second DOWN is blocked. All four
old tests were rewritten from ROM-measured values, not grid guesses.

## Verification
- `go test -count=1 ./...` (POKEMON_RED_ROM set) passes twice in a row.
- Verbose: all 5 movement tests PASS, ~0.01-0.05s each on the cached
  v2 fixture. First vertical slice complete: load ROM -> controllable
  overworld -> read map/position from RAM -> path -> move -> verify.

## Notes for next task
- Run tests with an ABSOLUTE POKEMON_RED_ROM path: `go test` runs from
  each package dir, so `roms/pokemon_red.gb` relative to repo root fails
  with "no such file or directory" for packages under skill/ and world/.
- Fixture cache lives at `skill/testdata/fixtures/reds_bedroom.v2.state`
  (gitignored). A stale fixture now self-heals (validated on load); if a
  movement test still says "blocked at (3,6)", delete
  `skill/testdata/fixtures/*` and re-run.
- Offsets unchanged: XCoord 0xD362, YCoord 0xD361, CurMap 0xD35E,
  CurMapWidth 0xD369, CurMapHeight 0xD368, WalkCounter 0xCFC5.
- Ground truth from (3,6) on map 0x26: UP blocked; DOWN (3,7); LEFT (2,6);
  RIGHT (4,6); second DOWN blocked at (3,7); LEFT,LEFT -> (1,6).
