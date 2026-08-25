# RUNNOTES — R1 collision fix (done)

## What changed
Commit "world: collide on the tile the player stands on" (7d2bdec).

- `world/grid.go`: Build now samples the step's BOTTOM-LEFT tile
  (`romData[tilesOff+(2*sy+1)*4+2*sx]`) instead of top-left (`2*sy`).
  The game collides on the tile under the player's feet. Comment updated
  to say this was measured against Oak's Lab, not assumed.
- `world/grid_test.go`: new `TestBuildOaksLabCollision` (ROM-gated,
  skips without POKEMON_RED_ROM). Builds map 0x28, asserts 22 measured
  coordinates incl. the regression pair (6,4) walkable / (6,3) not
  walkable (the table with the three Poke Balls).

## Verified
- Full suite green with `POKEMON_RED_ROM=... go test ./... -skip
  TestGoToViridianPokecenter` (that test stays red until the plan's last
  task by design).
- Regression check done: with the index flipped back to `2*sy`, the new
  test fails at exactly (0..3,2) and (6..8,4) — the measured disagreement.
  Flipped forward again; green.
- FindPath, Grid shape, tileset table layout untouched. skill/, red/,
  emu/ untouched.

## Must know for next task
- The collision grid now matches the game on Oak's Lab; S3-6's pathfinding
  blocker is fixed. If a route still fails, it is NOT a Build indexing bug —
  check NPC sprites (Grid does not model them) or the pathfinding input.
- `TestBuildOaksLabCollision` is the guard: any change to Build's tile
  indexing must keep it green.
- `world/grid.go` and `world/graph.go` have a pre-existing gofmt misalignment
  (tilesetEntryLen const / graph.go); left as-is to keep diffs surgical.
- Battle skill from S3-5 still applies for S3-6: `Battle(e,
  skill.FirstUsableMove)` after driving the encounter; main menu
  `wMaxMenuItem==1`, move menu `>=2` (1-indexed cursor); no faint/switch
  handling — use a mon that wins.
- ROM: /home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb
  (also: /home/maestro/Downloads/Pokemon - Red Version (USA, Europe)
  (SGB Enhanced).gb). Export POKEMON_RED_ROM to run ROM-gated tests.
