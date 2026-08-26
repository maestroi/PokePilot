# RUNNOTES — Travel re-reads the settled world after every battle (2026-08-26)

## What changed
- skill/travel.go: `Travel` now settles the world after every battle before
  the next leg is planned: `settleWorld` steps frames until
  (wCurMap, XCoord, YCoord) has been unchanged for 100 consecutive frames
  (budget 1200), and after a loss first waits for wCurMap to change. The
  map-change wait is the point: a blackout lands the position on the
  center's spawn tile while wCurMap still holds the old map, and that
  pre-flip window is itself stable, so a plain stability wait settles on
  the stale map (the measured "step down blocked at (5,6)" that walked a
  0x0C plan while on 0x00). `TravelResult` gains `Replans []Replan` (map +
  tile re-read after each battle, in order) so the re-plan is observable
  from outside the package.
- skill/travel_test.go: TestTravelReplansFromTheWorldAfterEachBattle —
  post_starter -> viridian city; asserts the battle happened (>= 1), one
  Replans entry per battle, every re-read on 0x0C, the first re-read
  exactly (0x0C, 14, 7) (the encounter tile a win leaves the player on,
  measured: battle fires stepping from (14,7) into (14,6), win leaves the
  player on (14,7)), and arrival on 0x01. Uses post_starter, not the
  literal pallet_town fixture: from the pallet_town checkpoint's frame
  phase (18257) Route 1's grass throws zero encounters deterministically,
  while post_starter's phase (18121) fights one at (14,7) — so the
  pallet_town start cannot produce the battle this test observes (the task
  text's "it reliably does" is measured the other way; both checkpoints
  are Pallet Town, post_starter being Oak's lab).

## Red/green verification (done)
- Reverted skill/travel.go to HEAD with the new test in place:
  `go test ./skill/ -run TestTravelReplansFromTheWorldAfterEachBattle`
  FAILED to build — "res.Replans undefined (type skill.TravelResult has no
  field or method Replans)". Red: the broken code has no observable
  re-plan, so the observation cannot even be expressed against it.
- Restored the fix: same command PASSES (2.0s). TestTravelPalletToViridian
  still passes (1 battle, arrives 0x01); TestGoToViridianPokecenter still
  passes (0 battles from the pallet_town phase — no settle path hit, so
  its timing is unchanged).
- Full gate: `go build ./... && go vet ./... && go test ./... -count=1`
  with POKEMON_RED_ROM set -> all packages ok, 0 failures, 0 skips.

# RUNNOTES — S5-2: five named places (Route 2, Forest, Pewter x3)

## What changed
- skill/goto.go: added to `places` (each verified InBounds+Walkable, none on
  an object's home tile): "route 2" 0x0D (8,71) — south-edge open band
  (x7-9), landing zone of the crossing from Viridian's north edge (x17-19).
  "pewter city" 0x02 (14,8) — plaza directly below the center door warp
  (14,7). "pewter pokemon center" 0x34 (2,4) — 0x34 derived from 0x02's
  warps (center doors (14,7)+(19,5) -> 0x34; gym door (16,17) -> 0x36,
  matching the measured gym id); nurse sprite 11 at (1,4), stand is the
  open floor beside her, mirroring the live-verified 0x29 pattern. "pewter
  gym" 0x36 (4,2) — Brock sprite 12 at (4,1), stand directly below him.
  "viridian forest" 0x33 (17,43) — open southern floor; (16,43) is a
  standing NPC's home tile, so the stand is one tile east of it.
- skill/goto_test.go: TestPlaceDestinationsStandable iterates
  skill.PlaceNames() (no hand-written list); per name: rom.ParseMap +
  world.Build, assert InBounds && Walkable, and that the destination is not
  an object's home tile. Skips only when POKEMON_RED_ROM is unset.

## Measured (POKEMON_RED_ROM = /home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb)
- go test ./... -count=1: exit 0, all ok; -v: 164 PASS, 0 FAIL, 0 SKIP.
- Center/gym stand tiles could NOT be live-walked this slice (Pewter is
  unreachable, below); they rest on grid + object geometry matching the
  live-verified 0x29 (nurse) pattern exactly.

## Findings for the next task
- PEWTER IS UNREACHABLE in the current game state (post-starter). 0x0D is
  adjacent only to 0x01/0x02; 0x02 only to 0x0D/0x0E (0x0E->0x0F->0x03 is
  the far side). The 0x01->0x0D crossing (city north corridor x17-19) is
  blocked: stepping north from (19,10) fires "You can't go through here!"
  (sprites 72 at (18,9) and 13 at (17,9); not trainers, text ids 4/5); the
  box re-fires on every retry. Detour rooms 0x2c/0x2d (warps (21,9),
  (32,7)) and the west chain 0x01->0x21->0x22->0x09 are dead ends. Measured
  live 3x.
- So the badge route to Pewter needs the story event that lifts the
  corridor first (candidate: an NPC or house in Viridian's south half);
  until then Travel/GoTo to 0x0D/0x02 fails with ErrDialogueInterrupted.
- Forest 0x33 has no boundary edges; reachable only via houses 0x2f (warp
  0x0D (3,11)) and 0x32 (0x0D (3,43)), which exit into the forest by warp.
