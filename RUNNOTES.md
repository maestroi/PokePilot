# RUNNOTES — S5c-4: Decode live sprite positions from sprite RAM

## What changed
- `red/state/sprite.go` (NEW): `SpriteState{Slot int; X,Y int; PictureID uint8}` and
  `DecodeSprites(m *Mem) []SpriteState`. Returns live map objects in slots 1..15, in slot
  order. Slot 0 (the player) is never returned. Layout constants are unexported/local.
- `red/state/sprite_test.go` (NEW): synthetic, package-internal. Proves slot 0 excluded when
  active, data1[0]==0 skipped, data1[0x02]==0xff skipped, slot 15 included (upper bound), and
  a live slot with the data2+0x0d scratch byte zeroed is still included (regression guard).
- `skill/sprite_fixture_test.go` (NEW): real-ROM anchor. Loads viridian_pokecenter (map 0x29),
  snapshots RAM, ParseMap(0x29), and compares decoded slot 1 (the nurse) against
  header.Objects[0]. ParseMap already strips the +4 bias, so they must agree exactly.

## The liveness predicate (the part an earlier draft got wrong)
Liveness comes from **wSpritePlayerStateData1 (0xC100)**, NOT wSpriteStateData2+0x0d (that
byte is scratch; map_sprites.asm zeroes every slot after tile patterns load, so reading it
returns nothing):
    data1 = 0xC100 + slot*0x10 ; data2 = 0xC200 + slot*0x10
    live  = data1[0x00] != 0 && data1[0x02] != 0xff
    Y = data2[0x04] - 4 ; X = data2[0x05] - 4
data1[0x00]=PICTUREID (0 = unused slot, zeroed at map load); data1[0x02]=IMAGEINDEX
($ff = hidden/removed, e.g. a picked-up item ball keeps a non-zero picture ID). The -4 is the
ROM's +4 bias (home/overworld.asm copies map-object Y/X straight into data2[4]/[5]).
wNumSprites is not needed: unused slots are zeroed at map load.

## Verified
- go test ./... -skip TestGymBoulderBadge (POKEMON_RED_ROM set): all pass, 0 fail. Only skip is
  TestProbe's permanent PROBE_MAP gate.
- Anchor genuinely runs: on the committed fixture, decoded slot 1 = pic 0x29 (3,1) == header
  Objects[0]. Only 2 of the pokecenter's 4 header objects are live (the other two are IMAGEINDEX
  $ff / not rendered), so the predicate genuinely discriminates.

## For S5c-5a/b (wiring into pathfinding)
- Decoder only; NOT wired into walkAround/GoTo/warp/story yet. Rebuild blockers from a fresh
  DecodeSprites snapshot each plan — no blocker cache (see AGENTS.md: bans that outlive a plan are bugs).
- Coordinates are tile coords on the current map, same space as skill.Place / FindPath.
- TestGymBoulderBadge stays excluded from the gate (S5c-6 owns it; rDIV-seeded battles).
