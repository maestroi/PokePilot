# RUNNOTES — S2-7 fixtures (BLOCKED: emulator freeze on Pallet Town north edge)

## Status
BLOCKED. `pallet_town` fixture is producible (arrival state, no crossing).
`viridian_city` + `viridian_pokecenter` require crossing Pallet Town's north
edge (map 0x00 -> Route 1 0x0C), which hangs. S2-6 GoTo work is ported,
builds, vets, and all skill tests pass except `TestGoToViridianPokecenter`.

## Root cause (isolated, reproducible)
Stepping the player UP from row y=1 into row y=0 of Pallet Town makes the
game set `wJoyIgnore` (0xCD6B) to `0xFC` within 1 frame, which freezes the
player in ALL 4 directions, persistently. Reproduced at (11,1) and (10,1).
- No map transition is in progress: `wFadeoutMode` (0xD838) = 0x00,
  `wMapStatus` (0xD828) = 0x00, map stays 0x00, player never moves.
- Map dims are correct: `wCurMapWidth`=10, `wCurMapHeight`=9 (blocks).
- Target tile (11,0)/(10,0) = 0x52, which IS in the tileset-0 walkable list.
- The write to 0xCD6B is indirect (no `ld [0xCD6B],a` in the ROM).
- The player CAN step (11,2)->(11,1); only the step INTO row y=0 is blocked.
This is a gomeboy/game behavior, not pokepilot code. The crossing is
impossible, so the two fixtures that need it cannot be produced.

## Repro (scratch2/main.go, deleted before commit)
Boot -> walk to Pallet Town (5,6) -> WalkPath to (11,2) -> StepOnce Up to
(11,1) -> Press Up. wJoyIgnore goes 0x00 -> 0xFC at frame 1, player frozen.

## Why not fixed here
- Not in `world/`, `red/`, or `skill/` (those are correct; task forbids
  changing world/ and red/ anyway).
- The freeze is the ROM's own code running under gomeboy. A fix means
  changing the gomeboy emulator (separate repo) or understanding a Gen 1
  edge/transition mechanic. Out of scope for this pokepilot task.

## Next task options
1. Fix gomeboy (separate repo) so stepping into the connection edge row
   works, then re-run S2-7.
2. Scope S2-7 down to the `pallet_town` fixture only (no crossing needed)
   and defer the two crossing fixtures.
3. Find an alternate route that avoids Pallet Town's north edge (none known;
   the only route to Viridian is 0x00 -> 0x0C -> 0x01).

## Verification done
`go build ./...`, `go vet ./...` clean. `go test -count=1 ./...` green except
`TestGoToViridianPokecenter` (the crossing). ROM:
/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb.
