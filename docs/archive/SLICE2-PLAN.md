# Slice 2 draft plan — interaction + cross-map navigation

Status: **measured and published to agent-runner.**

Goal: `go_to("Viridian Pokémon Center")` then talk to the nurse, entirely
deterministically. Route: Red's bedroom -> Red's House 1F -> Pallet Town ->
Route 1 -> Viridian City -> Pokémon Center -> nurse.

Prerequisite: Phase 1, committed at `be1d7b5`. Full suite green.

---

## Ground truth (already measured — do not re-guess)

Parsed with our own `red/rom.ParseMap` against the real ROM, so the parser is
confirmed working on every map in this route. Coordinates are game tile
coordinates, the same ones `wXCoord`/`wYCoord` report.

| Map | ID | Blocks | Warps |
|---|---|---|---|
| REDS_HOUSE_2F | `0x26` | 4x4 | (7,1) -> `0x25` warp 2 — the stairs |
| REDS_HOUSE_1F | `0x25` | 4x4 | (2,7) and (3,7) -> `0xFF`; (7,1) -> `0x26` warp 0 |
| PALLET_TOWN | `0x00` | 10x9 | (5,5) -> `0x25`; (13,5) -> `0x27` Blue's house; (12,11) -> `0x28` Oak's lab |
| ROUTE_1 | `0x0C` | 10x18 | none — 2 connections only |
| VIRIDIAN_CITY | `0x01` | 20x18 | (23,25) -> `0x29` Pokémon Center; plus mart/school/house/gym |
| VIRIDIAN_POKECENTER | `0x29` | 7x4 | (3,7) and (4,7) -> `0xFF` (exit) |

**The nurse** is Viridian Pokémon Center object 0: tile **(3,1)**, sprite 41,
text id 1. Counter is between her and the player, so the interaction is
"stand below and face up", not "stand on her tile".

**Player start** (from the `reds_bedroom` fixture): map `0x26`, (3,6). `UP` is
blocked; `DOWN`/`LEFT`/`RIGHT` open; `y=7` is the bottom row.

### Measured by driving the real ROM (2026-08-24)

Probe route: boot -> walk 2F (3,6) to (6,1) -> push RIGHT -> arrive 1F (7,1) ->
walk to (3,2) -> face UP -> A on the TV sign. All of this works today with
Phase 1 primitives only.

1. **Warp tiles are not walkable.** Tileset collision marks every door and
   staircase solid: 2F (7,1) and 1F (2,7)/(3,7) all come back solid from
   `world.Build`. You take a warp by walking *into* the warp tile from an
   adjacent walkable tile. A* must route to the neighbour, then push.
2. **After a warp the player stands on a solid tile.** Arrival on 1F is (7,1),
   which that map's own grid calls solid, so `FindPath` from there returns
   `ErrNoPath`. The pathfinder must always force the start tile walkable.
3. **`wPlayerDirection` (0xD52A) is a bitmask, not a sprite facing.**
   `PLAYER_DIR_RIGHT=1, LEFT=2, DOWN=4, UP=8`. The `SPRITE_FACING_*` values
   (0/4/8/12) that `red/state.Facing` uses live at **0xC109**
   (`wSpritePlayerStateData1 + 9`). Phase 1 decodes the wrong address; see S2-0.
4. **`wTextBoxID` is useless as an open/closed signal.** It read `0x01` before,
   during and after the dialogue. `wFontLoaded` is the signal.
5. **A-press count to close a dialogue is timing-dependent**: the same TV sign
   took 10 presses at a 40-frame cadence and 6 at a 100-frame cadence. `Talk`
   must be a bounded poll on `wFontLoaded`, and no test may assert a count.

---

Two facts that shape the design:

1. **`DestMap == 0xFF` means "the map you came from"**, not map 255. Every
   building exit uses it. The warp graph must resolve it from context
   (`wLastMap`), not treat it as a real map id.
2. **Route 1 has no warps at all.** Pallet Town -> Route 1 is a map
   *connection*, a different mechanism from a warp: you walk off the map edge.
   Cross-map routing needs both edge types or the route cannot be found.

---

## Tasks (published as plan 47dd723a-d4cf-443f-b53d-41b21db7612f)

| | Task | First edit | Depends |
|---|---|---|---|
| S2-0 | Fix the facing decoder — read 0xC109, not 0xD52A | `red/sym/addresses.go` | — |
| S2-1 | Pathfinder: solid start tiles, `FindPathAdjacent` | `world/path.go` | — |
| S2-2 | Map graph over warps and connections, `0xFF` resolved | `world/graph.go` | — |
| S2-3 | BFS route search across maps | `world/route.go` | S2-2 |
| S2-4 | `Face` and `Talk` | `skill/interact.go` | S2-0, S2-1 |
| S2-5 | `Traverse` one edge (warp or connection) | `skill/warp.go` | S2-1, S2-2 |
| S2-6 | `GoTo` — the milestone, replans from RAM each leg | `skill/goto.go` | S2-3, S2-5 |
| S2-7 | Fixtures for the new checkpoints, `fixtureVersion` 3 | `skill/fixture/fixture.go` | S2-6 |

Design choices baked into the tasks:

- The graph is **map-level**, not tile-level. Arrival coordinates are never
  computed from the connection struct's alignment fields; they are read back
  from RAM after the transition and the route is replanned from there. That
  deletes a whole class of arithmetic bugs.
- A warp edge is "path to a walkable neighbour of the warp tile, then push",
  because warp tiles are solid.
- `GoTo` does not fight. A battle aborts with `ErrBattle`; slice 3 adds
  execution and retry.
- Named destinations are a Go map literal.

## Process notes

- **Measure ground truth before writing task text.** Both Phase 1 failures were
  my wrong assumptions, not the local model's coding. Every "MEASURED" block in
  the published tasks came from driving the real ROM on 2026-08-24.
- **Every "it failed" assertion must also assert nothing changed.** See
  DESIGN.md 3.2b.
- One task per commit-sized outcome, `First edit:` with an exact path, at most
  six file directives, verification command pre-seeded with
  `POKEMON_RED_ROM=/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb`.
