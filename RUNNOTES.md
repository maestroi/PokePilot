# RUNNOTES — S2-4 Face and Talk (DONE, this commit)

## What I changed
- `skill/interact.go` (new):
  - `Face(m, tx, ty) error` — taps the direction toward the adjacent tile,
    StepUntil (60 frames) until DecodePlayer reports the expected facing.
    Rejects non-adjacent tiles. Asserts facing, never position: a tap
    toward an open tile may move the player, which is fine.
  - `Talk(m) (int, error)` — taps A, StepUntil (120 frames) for
    FontLoaded != 0 (else ErrNoDialogue), then a bounded poll: while
    FontLoaded != 0, tap A + StepFrames(40) settle, capped at 30 presses
    (error names the cap). On close, waits (40 frames) for
    state.Controllable — wJoyIgnore can clear a few frames after
    wFontLoaded — and returns the press count.
  - Budgets: faceTurn 60, talkOpen 120, talkSettle 40, talkPressCap 30.
    Settle keeps the A cadence (~50 f/press) in the cheaper regime
    (measured: 10 presses @40f cadence, 6 @100f).
- `skill/interact_test.go` (new):
  - TestFaceEachDirection: from reds_bedroom, Face all four adjacent tiles
    in turn; decoded facing asserted each time; player never moved.
  - TestFaceRejectsNonAdjacentTile: Face(1,1) errors.
  - TestTalkTVSign: measured probe route — FindPath 2F (3,6)->(6,1) +
    WalkPath; HoldUntil(Right, 120) until CurMap != 0x26 (stairs, lands
    1F (7,1)); FindPath 1F (7,1)->(3,2) (solid start, S2-1 fix) + WalkPath;
    Face(3,1); Talk(). Asserts count >= 1, no error, Controllable after,
    coords still (3,2). No press-count assertion. Helpers: gridFor
    (rom.ParseMap + world.Build from e.ROM()), controllable; loadFixture/
    playerAt reused from move_test.go.

## Verification
- `go build ./...`, `go vet ./...` clean; `go test -count=1 ./...` green WITHOUT POKEMON_RED_ROM (skips) and WITH (all ok, skill 5.6s).

## Notes for next task (S2-5 Traverse)
- Stairs pattern: HoldUntil(direction, 120, CurMap != from) works; the
  button is released the frame the map flips, so the player does not
  re-walk on the destination map.
- After a warp the player stands on a solid tile (1F (7,1)); FindPath's
  start-tile fix handles it. Arrival coords must be read from RAM.
- Keep any new interaction primitive on FontLoaded, never TextBoxID; fixtureVersion still 2, reds_bedroom fixture unchanged.
