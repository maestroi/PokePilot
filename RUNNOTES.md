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
