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

## Tasks

### S2-0 — Fix the facing decoder

Goal: `red/state.Facing` reports the real sprite facing.

First edit: Modify `red/sym/addresses.go` to add
`SpritePlayerFacing uint16 = 0xC109`.

Then change `DecodePlayer` to read facing from that address, and add
`PlayerDirection`-bitmask constants only if something needs them. Verified
values while walking: down=0, up=4, left=8, right=12.

Tests: from the `reds_bedroom` fixture, tap each direction and assert the
decoded `Facing`. This currently fails, which is the point.

Depends on: nothing.

### S2-1 — Warp and connection graph

Goal: a graph over all maps where nodes are (map, tile) and edges are warps
plus edge connections, so a single search can cross maps.

First edit: Create `world/graph.go`.

- `type Node struct { Map uint8; X, Y uint8 }`
- `func BuildGraph(rom []byte) (*Graph, error)` — parse every map, add warp
  edges, resolve `0xFF` destinations by back-reference (if map A warp W leads
  to `0xFF`, the return edge is whichever warp in the parent map points at A).
- Connection edges: the 11-byte connection blocks in the map header carry the
  connected map plus an offset; decode them into edges along the shared border.
  Read `/home/maestro/.cache/pokered/macros/data.asm` (the `connection` macro)
  for the field layout rather than guessing.

Warp tiles are solid (see measurement 1), so an edge is "stand on the
walkable neighbour of the warp tile, push into it". Model the edge that way,
not as "walk onto the warp tile".

Tests: `0x26` reaches `0x25` via (7,1); `0x25` reaches `0x00`; `0x00` reaches
`0x0C` by connection; `0x01` reaches `0x29`. Graph builds for all ~248 maps
without error. ROM-gated, skip without `POKEMON_RED_ROM`.

Depends on: nothing (Phase 1 `red/rom` is enough).

### S2-2 — Cross-map route search

Goal: `FindRoute(g *Graph, from, to Node) ([]Leg, error)` where a `Leg` is
either "walk this path on this map" or "take this warp/connection".

First edit: Create `world/route.go`.

Force the start tile walkable before every within-map search (measurement 2)
and target the warp tile's neighbour, not the warp tile.

Reuse the existing per-map A* for within-map legs; search the map graph for the
sequence of edges. Do not write a second pathfinder.

Tests: route from `{0x26,3,6}` to `{0x29,3,2}` (in front of the nurse) exists
and its legs are ordered and contiguous — each leg starts where the previous
one ended. Pure, no emulator.

Depends on: S2-1.

### S2-3 — Interaction skill

Goal: face a target tile and talk to it, advancing dialogue to completion.

First edit: Create `skill/interact.go`.

- `func Face(m *emu.Emu, tx, ty uint8) error` — turn toward an adjacent tile,
  verify the decoded `Facing` from S2-0. Pressing a direction you are not
  facing turns without moving, which is exactly what we want here.
- `func Talk(m *emu.Emu) (int, error)` — press A, verify `wFontLoaded != 0`,
  then keep pressing A while it stays non-zero, until it clears or the budget
  runs out. Returns the number of presses spent.

Ground truth is measured (see above): use `wFontLoaded`, never `wTextBoxID`,
and never assert a press count — it is timing-dependent.

Tests: from the `reds_bedroom` fixture, face each direction and assert the
decoded facing. Then the TV sign on Red's House 1F, tile (3,1), reachable from
(3,2) facing up: `Talk` opens a box, closes it within budget, and the player is
`Controllable` again afterwards with unchanged coordinates.

Depends on: S2-0.

### S2-4 — Warp execution

Goal: `TakeWarp(m *emu.Emu, w rom.Warp) error` — walk onto a warp tile and
confirm `wCurMap` changed.

First edit: Create `skill/warp.go`.

Measured: you stand on the walkable neighbour and push the direction of the
warp tile; the map changes mid-push. No separate "step again" is needed for
the 2F staircase. Do not path onto the warp tile itself — it is solid. Verify by `wCurMap` change with a frame
budget, then wait for the new map's dimensions to be non-zero — the same
positive-fact rule that fixed Phase 1 (see DESIGN.md 3.2b).

Tests: from `reds_bedroom`, route to (6,1), take the stairs, assert
`wCurMap == 0x25`, position (7,1), dimensions non-zero, and `Controllable`.

Depends on: S2-2.

### S2-5 — go_to

Goal: the single entry point the planner will eventually call.

First edit: Create `skill/goto.go`.

- `func GoTo(m *emu.Emu, dest world.Node) error` — build/reuse the graph, find
  the route, then execute leg by leg: walk within a map, take the warp or
  connection, re-read state, and **replan from actual position** after each leg
  rather than trusting the plan.
- On `ErrBlocked`, retry with the blocking tile marked in the dynamic `blocked`
  set; after N failures return an error carrying the decoded state.
- Named destinations: a small table mapping strings like
  `"viridian pokemon center"` to a `Node`. Keep it a table, not a parser.

Tests: the milestone — from `reds_bedroom`, `GoTo` the tile in front of the
nurse, assert `wCurMap == 0x29` and the expected coordinates. Then `Talk`.
Expect this to be slow; it is the one long test and it earns its keep.

Depends on: S2-4.

### S2-6 — Fixtures for the new checkpoints

Goal: `pallet_town`, `route_1`, `viridian_city`, `viridian_pokecenter`,
`nurse_facing` fixtures, each generated by running `GoTo` once and snapshotting.

First edit: Modify `skill/fixture/fixture.go` to register them.

Bump `fixtureVersion`. Every fixture must pass the existing validation.

Depends on: S2-5.

---

## Process notes

- **Measure ground truth before writing task text.** Both Phase 1 failures were
  my wrong assumptions (an absence-only predicate, a wrong walk direction), not
  the local model's coding. The route data above was measured for exactly that
  reason. S2-3's dialogue counts are still unmeasured and marked as such.
- **Every "it failed" assertion must also assert nothing changed.** See
  DESIGN.md 3.2b.
- One task per commit-sized outcome, `First edit:` with an exact path, at most
  six file directives, verification command pre-seeded with
  `POKEMON_RED_ROM=/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb`.

## Decisions

1. `GoTo` does **not** fight. A wild battle aborts the route with a typed
   error; slice 3 adds battle execution and `GoTo` will then retry.
2. Named destinations live in a Go table, not a data file.
