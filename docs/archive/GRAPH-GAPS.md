# Why the map-level graph cannot route some legs

Measurement survey for S10-12. Every claim below was measured against
`roms/pokemon_red.gb` with a scratch BFS/flood-fill over `world.Build`
grids (`world/zz_gaps_test.go`, deleted before commit), or read from the
vendored decomp at `pokered/`. The ROM is byte-identical to the unmodified
Red (sha1 `ea9bcae617fdf159b045185467ae58b2e4a48b9a`), so "the original
game" and "this ROM" are the same bytes.

Three questions, each answered by measurement:

1. Do the phantom edges (legs 5, 7) and the S8-9 fragmentation share a root?
2. Are the 7 unexplained unreachable maps the same 0xFF-warp bug as `0xC1`?
3. What is the smallest change that lets a planner route legs 5 and 7?

Background: `docs/ROAD-TO-ELITE-FOUR.md` (S9-3) surveyed the Cerulean→Indigo
road. The map-level shortest path is 8 legs; legs 5 and 7 are **phantom
edges** (the header declares a connection, the tiles are walled) and leg 8 is
water-gated. This doc explains *why* the graph cannot route legs 5 and 7, and
what the smallest fix is. It does **not** write the fix (that is S10-12).

---

## Q1 — Do phantom edges and the S8-9 fragmentation share a root?

**Yes. The root is: the map is one node, but its walkable region is not
connected.** A phantom edge and a fragmented map are the same phenomenon at
different scales.

### The measurement

`world.Build` produces a per-map grid and a per-tileset walkable list.
Flood-filling the grid (4-directional) gives the walkable **components** of
each map. A map is one node in `world.BuildGraph`, but if its walkable region
splits into multiple components, the node is a lie: two tiles the graph
treats as "the same map" are not reachable from each other on foot.

- **128 of 228 parseable maps** have more than one walkable component.
- For legs 5 and 7, the entry and exit ports fall in **different components**:

| Leg | Map | Entry port (component) | Exit port (component) | Why it fails |
|-----|-----|------------------------|-----------------------|--------------|
| 5 | `0x0D` Route 2 | N edge → `0x02` (c0, size 86) | S edge → `0x01` (c10, size 245) | three full-width solid rows separate them — see below |
| 7 | `0x21` Route 22 | E edge → `0x01` (c0, size 180) | N edge → `0x22` (**SOLID**) | rows 0–1 fully walled; the N edge has no walkable tile at all |

### Reconciling Route 2: "row 22" and "rows 36-37" are both real

S9-3 (`docs/ROAD-TO-ELITE-FOUR.md:27,42`) reports Route 2 (`0x0D`, 20×72)
walled at **row 22**. S8-9 (`RUNNOTES.md:985-990`) reports it walled at
**rows 36-37**. These looked contradictory going into this task. Re-measured
directly: **both are correct — Route 2 has three separate full-width solid
rows, not one.**

```
row y=22  fully solid, width 20
row y=36  fully solid, width 20
row y=37  fully solid, width 20
```

The map flood-fills into **11 components** (sizes 86, 146, 8, 25, 16, 63,
≤5, 8, 17, 189, 245). Component 0 (size 86) sits north of row 22 and holds
the Pewter-side entry port; component 10 (size 245) sits south of row 37 and
holds the Viridian-side exit port — they were never going to be the same
component, since at least two independent full-width walls stand between
them. The exact banding of the other 9 components (buildings, the mid-map
strip between rows 23-35) was not required to answer Q1 and is left
UNMEASURED. **Either wall alone already breaks the direct N→S walk; the map
happens to have three.** S9-3 and S8-9 were not in conflict — they cited
different barriers on the same map, and neither doc claimed to have found
all of them.

### Route 4: the real wall is columns 20-23, not the map edge

Re-measured Route 4 (`0x0F`, 90×18): **5 components**. Solid columns:

```
col x=0..3    fully solid, height 18   (map edge / border tiles)
col x=20..23  fully solid, height 18   (the mountain wall S8-9 measured)
```

This matches S8-9's finding (`RUNNOTES.md:991-993`) exactly: columns 20-23
are the genuine west/east split. Columns 0-3 are the map's own border and
are not a routing-relevant barrier (no port lands there).

### The forest is not fragmented — confirms S8-9, does not contradict it

Re-measured Viridian Forest (`0x33`, 34×48): **exactly 1 component** (size
719, all tiles). This directly confirms S8-9's own finding
(`RUNNOTES.md:994-996`, "1 component under BL — correct"). The *task
premise* S8-9 was investigating — that the forest fragments — was the thing
S8-9 disproved; this measurement reproduces that disproof, it does not
overturn a conclusion S8-9 reached. The forest is walkable end to end
(the measured 130-step path from `(2,1)` to `(16,47)` in S9-3 is inside this
one component).

### Mt Moon 1F: a separate, smaller fact than the S8-6 misattribution

Re-measured Mt Moon 1F (`0x3B`, 40×36): **3 components** — one main region
(size 1076) and two small pockets (37 and 31 tiles, 6% of the walkable area
combined). This does not contradict S8-9's Mt Moon finding: S8-9 addressed
one specific claim (that tile `(9,22)` was blocked by collision) and showed
that tile is walkable and unblocked — a misdiagnosis of *that* tile, not a
claim that Mt Moon 1F has zero components. The two small pockets are a real,
separate fact S8-9 did not measure (it was not asked to). Neither pocket sits
on the Cerulean→Indigo road (Mt Moon is not one of the 8 legs), so this does
not change the leg 5/7 fix, but it means "the map fragments" is true of Mt
Moon 1F for reasons unrelated to the (9,22) question S8-6/S8-9 argued about.

### The "routing-relevant split" is rarer than it looks

A map is a *routing problem* only if it has ≥2 entry ports **and** ≥2 exit
ports **and** no single component holds both an entry and an exit. Measured:
**0 maps** meet all three criteria. So no map is "impossible to cross" in the
strong sense. The legs 5/7 failures are the weaker case: the *specific*
entry and exit the planner needs happen to be in different components, even
though other entry/exit pairs on the same map are fine. This matters for the
fix: the graph does not need to *remove* the map, it needs to *route
around* the split.

---

## Q2 — Are the 7 unreachable maps the 0xFF-warp bug?

**No. None of the 7 is the 0xFF-warp bug.** They are dead data that
`world.BuildGraph` is correct to exclude.

### The measurement

228 maps parse; 221 are reachable from Cerulean (`0x03`). The 7 unreachable
are (hex → identity → why):

| Map | Identity (decomp) | Why unreachable |
|-----|-------------------|-----------------|
| `0x45` | `CERULEAN_TRASHED_HOUSE_COPY` | dead `_COPY` duplicate; no warp points at it |
| `0x4B` | `UNDERGROUND_PATH_ROUTE_6_COPY` | dead `_COPY`; the live map is `0x4A` (its warps go to `0x4A`, not `0x4B`) |
| `0x4E` | `UNDERGROUND_PATH_ROUTE_7_COPY` | dead `_COPY`; the live map is `0x4D` (its warps go to `0x4D`, not `0x4E`) |
| `0xAD` | `CINNABAR_MART_COPY` | dead `_COPY`; the live map is `0xAC` |
| `0xE7` | `UNUSED_MAP_E7` | garbage/unused header; no warp points at it |
| `0xEF` | `TRADE_CENTER` | sealed room; `def_warp_events` is empty, no warp points at it |
| `0xF0` | `COLOSSEUM` | sealed room; `def_warp_events` is empty, no warp points at it |

All seven are unreachable **in-game** as well (no script or warp enters
them), so the graph builder is not wrong to leave them out. The `_COPY` maps
(`0x45`/`0x4B`/`0x4E`/`0xAD`) are dead duplicate maps present in the base
game — they exist in `pokered/constants/map_constants.asm`, byte-identical
to unmodified Red, but no warp or script in the vanilla game ever targets
them. They are not artifacts of this ROM; they are unused data the original
developers left in.

### The 0xFF-warp bug is a different thing

`0xC1` (Route 22 Gate) is **reachable** but is a **dead end** in the graph.
Its warps all point at `0xFF` ("the map you came from"):

- The game resolves `0xFF` at runtime via `wLastMap` (the map the player
  came from). `Route22Gate.asm` sets `wLastMap` by Y position
  (`Y<4 → ROUTE_23`, `Y≥4 → ROUTE_22`), so the `0xFF` warps work in-game.
- The **static** graph builder cannot model script behavior, so it resolves
  `0xFF` as "no target" and drops the edge. `0xC1` becomes a leaf.

So the 0xFF-warp bug is a **resolver limitation** (the static builder cannot
see through a script-computed target), not a data bug. The 7 unreachable maps
are **data** (dead duplicates / sealed rooms), not a resolver limitation.
Different root, different fix.

---

## Q3 — Smallest change to route legs 5 and 7

Three approaches were considered for the leg 5 family of failures (any map
whose declared connection lands in the wrong component). Each is evaluated
on cost, what it breaks, and what it does **not** solve, since none of them
is free.

### Option A — component-aware leg predicate (recommended)

When the planner considers walking from port A to port B *within* a map,
check A and B are in the same walkable component (computed once per map,
cached). If not, reject that leg and let search fall back to the warp
detour.

- **Cost:** one predicate function plus a per-map component cache (~one
  flood-fill per map, done once at graph build time). No new node/edge
  types, no change to `world.BuildGraph`'s output shape.
- **Breaks:** nothing — it only *removes* edges the graph currently offers
  incorrectly. Any caller that already handles "no path found" is
  unaffected.
- **Does not solve:** leg 7. Leg 7 fails because the only exit is a
  dead-end node (`0xC1`), not because two components are being conflated —
  rejecting a same-map walk does not create the missing edge through the
  gate. Needs Option A's sibling fix (the resolver change below) or Option
  B.

### Option B — components as graph nodes

Give each map one graph node **per walkable component**, instead of one
node per map. `world.BuildGraph` would gain nodes and the edges between them
would be exact.

- **Cost:** larger. Every place that identifies a location by map ID alone
  (skills, `GoTo`, logging, run reports) now needs a component index too.
  The component count itself must stay in sync with `world.Build`'s
  sub-tile rule — any future change there silently changes the graph's node
  count.
- **Breaks:** any code that treats "map ID" as a stable, sufficient key —
  measured as widespread in this codebase (`GoTo` by map ID, `Knowledge`
  entries keyed by map, run scoreboards). This is a structural graph
  rebuild, not a local patch.
- **Does not solve:** leg 7's dead end either — `0xC1`'s warps still resolve
  to nothing without the resolver fix, no matter how the rest of the graph
  is nodalized.

### Option C — planner learns the detour, graph left alone

Leave `world.BuildGraph` as-is and let the LLM planner discover the forest
detour by exploration, the way a live run already crossed it once.

- **Cost:** looks free in code, but is not free in practice — it spends
  planner rounds and tokens rediscovering a fixed piece of geometry every
  run, and S9-12 already measured the project's actual failure mode as
  rounds burned on bad reasoning under budget pressure. This is the
  opposite of cheap.
- **Breaks:** nothing in the graph, but conflicts with the project's
  stated architecture (`project goal`, "deterministic Go code executes
  intent" / "NEVER a screenshot-and-guess agent") — the graph is supposed
  to be ground truth the planner can trust, not a maze the LLM re-solves
  every run.
- **Does not solve:** leg 7's structural dead end, and does not fix the
  phantom edge for any *other* deterministic caller of the graph (a future
  skill that plans a route without an LLM in the loop still gets the wrong
  direct edge).

### Recommendation

**Option A (component-aware predicate) for leg 5, plus a resolver fix for
leg 7 (below).** Reject Option B as disproportionate to two known legs —
its blast radius (every map-ID-keyed caller) is not justified yet. Reject
Option C as contrary to the project's deterministic-routing architecture
and already shown expensive in round-budget terms.

**What would change this:** if a future slice needs *many* legs fixed this
way — not two — the per-leg patches in Option A start to look like the
component-as-nodes rebuild done piecemeal, and Option B's one-time cost
could become cheaper in aggregate. The "128 of 228 maps have >1 component"
figure is the number to watch: if routing failures start showing up on a
meaningful fraction of those maps (not just Route 2 and Route 22), revisit
Option B.

### Leg 5 (`0x0D` S → `0x01`): component-aware routing

The player enters `0x0D` at the N edge (c0, from Pewter) and must exit at the
S edge (c10, to Viridian). c0 and c10 are disconnected by Route 2's walls.
The graph currently treats `0x0D` as one node, so it offers the S edge as a
direct exit — a phantom edge.

The real crossing is the forest detour, which **is already in the graph**:
`0x0D`(S side) → warp `(3,43)` → `0x32` (south gate) → `0x33` (forest) →
warp `(1,0)/(2,0)` → `0x2F` (north gate) → `0x0D`(N side) warp `(3,11)`. The
planner has the pieces; it does not know the direct S edge is walled.

**Smallest change:** Option A above — reject same-map legs whose ports are
in different components, and let search fall back to the warp detour.

### Leg 7 (`0x21` N → `0x22`): resolve the gate's 0xFF warps

The N edge of `0x21` is fully walled (rows 0–1 solid). The only exit is the
gate building `0xC1`, whose warps point at `0xFF`. The graph drops those
edges, so `0xC1` is a leaf and the planner cannot route *through* it.

**Smallest change:** resolve the `0xFF` warps of `0xC1` (and any other
`0xFF`-warp building) to their actual targets. The targets are knowable from
the decomp: `Route22Gate.asm` sets `wLastMap` by Y, so the south door
(`Y≥4`) resolves to `ROUTE_22` (`0x21`) and the north door (`Y<4`) resolves
to `ROUTE_23` (`0x22`). Concretely, the gate's warps to `(7,139)/(8,139)` on
`0x22` are the exit the planner needs. Once `0xC1` is a pass-through node
(`0x21` → `0xC1` → `0x22`), leg 7 routes.

- **Cost:** one rule in the graph resolver, specific to `wLastMap`-style
  `0xFF` warps (a known, named pattern, not a heuristic).
- **Breaks:** nothing — it only resolves edges the builder currently drops.
- **Does not solve:** the gate-door finding below, which is a separate,
  open question about whether the door is reachable on foot at all.

### The gate-door finding (critical, needs S10-12 verification)

The gate door is at Route 22 `(8,5)`. **The grid model measures the door as
walkable but SEALED from the road.** This is a new finding (S9-3 did not
measure `(8,5)` reachability) and it matters for leg 7: if the door is truly
unreachable on foot, the planner cannot walk to it even after the 0xFF fix.

The measurement (Route 22 grid, 40×18):

```
    0123456789012345678901234567890123456789
y 0 ########################################
y 1 ########################################
y 2 ##############################......####
y 3 #################################.######
y 4 ################....................####
y 5 ########.#######....................####   <- door (8,5) is the lone '.' at col 8
y 6 ##............##......########....##....
y 7 ##............#################.###.....
y 8 ##............##......########....#.....
y 9 ###########.####......########....#.....
y10 ##............##..........####....#...##
y11 ##....##########..........####....#...##
y12 ##........................####........##
y13 #################################.######
y14 ##....................................##
y15 ##....................................##
y16 ########################################
y17 ########################################
```

- **c0 (size 180):** the main road (rows 4–5, cols 16–35), the east entry
  (cols 35–39, rows 6–9), the east corridor, and the bottom road (rows 14–15).
  The player enters from the east (Viridian) into c0.
- **c1 (size 110):** the door `(8,5)` and the upper-left pocket (cols 2–13,
  rows 6–10, plus cols 16–25 rows 10–12 and row 12 cols 2–25).
- **The seal:** row 12 cols 26–29 are solid, and row 13 is solid except the
  single gap at col 33 — which opens onto the c0 side (the east corridor),
  not the c1 side. The door `(8,5)` connects down to `(8,6)→(8,10)` but that
  pocket is cut off from the road by the row-7 wall (cols 16–29) and the
  row-12/row-13 walls.

The door's own block (`0x3A`) is 15/16 solid — only the bottom-left tile
`(r3,c0)` is passable, which is exactly the sub-tile `grid.go` uses, so the
door is *walkable* (the player can stand on it) but *unreachable* (no walkable
path from the road). The sealed result is **robust to the collision sub-tile
choice**: all four sub-tile options (top-left / bottom-left / top-right /
bottom-right) were tested and none connect the door to the road.

**This is the original layout, not a rearrangement bug.** The block data,
warps, and connections for map `0x21` are byte-identical between this ROM and
the unmodified Red. The decomp even annotates the N connection
`connection north, Route23, ROUTE_23, 0 ; unnecessary`
(`pokered/data/maps/headers/Route22.asm`), suggesting the N seam was never the
intended crossing.

**The discrepancy:** the original Red is playable, so the player *does* reach
the gate. But the grid says the door is sealed from the road. One of these is
wrong, and S10-12 must resolve it **with the emulator** before relying on the
0xFF fix:

- If the grid is right, the player reaches the door by a method the static
  grid does not model (a script teleport, a ledge drop, or the gate is entered
  from the Route 23 side). The 0xFF fix alone will not make leg 7 route.
- If the grid is wrong (a latent collision bug), the 0xFF fix is sufficient
  and the grid needs correcting.

S9-3's road doc states the door is the exit without measuring `(8,5)`
reachability; this measurement supersedes that assumption.

### Summary of the two fixes

| Leg | Blocker | Smallest change | Type |
|-----|---------|-----------------|------|
| 5 | `0x0D` N/S edges in different components (walls at y=22, 36, 37) | component-aware leg predicate (Option A: reject A→B if A,B in different components; fall back to the forest warp detour) | graph predicate |
| 7 | `0xC1` warps point at `0xFF` (dropped) | resolve `0xFF` warps of `0xC1` to `0x21`/`0x22` via the `wLastMap` rule | resolver |
| 7 (open) | gate door `(8,5)` measured sealed from road | verify with emulator; if sealed, find the real entry method | **open** |

---

## Method notes

- **Grids** came from `world.Build` (per-tileset walkable lists). The
  OVERWORLD walkable list (19 tiles) and the PLATEAU list match the ROM and
  the decomp byte-for-byte.
- **Coordinate system** (definitive, no factor-of-2 bug): `rom.MapHeader`'s
  `WidthBlocks`/`HeightBlocks` are the raw header bytes, which the decomp
  calls "map width/height in **4×4 tile** blocks"
  (`pokered/home/overworld.asm:2254,2257`, comments verbatim). The game
  doubles that into `wCurrentMapWidth2`/`wCurrentMapHeight2` — "map
  width/height in **2×2 tile** blocks" (`pokered/home/overworld.asm:2256,2259`)
  — and `wXCoord`/`wYCoord` (the task's "TILES", per the S10-11 instructions'
  contract "a block is 2x2 tiles") are in this doubled unit.
  `world.Build`'s grid width is `WidthBlocks*2`, landing in exactly that same
  doubled unit — the same one `wXCoord`/`wYCoord` use. So the grid and the
  player's own position already share one resolution; there is no second
  factor of two to apply on top, and this doc's row/column numbers are
  directly comparable to `wXCoord`/`wYCoord`.
- **Collision sub-tile:** `grid.go` uses the bottom-left tile of each 2×2
  cell (`block[2*sy+1][2*sx]`). The decomp's
  `GetTileAndCoordsInFrontOfPlayer`
  (`pokered/engine/overworld/player_state.asm:257`) checks the destination's
  top-left tile. For the gate door the two disagree (top-left is solid,
  bottom-left is passable), but the *sealed* conclusion holds under both, so
  the sub-tile choice does not change the finding.
- **Unreachable maps** were identified by a full-ROM warp scan (which maps do
  any warps point at) cross-referenced with the decomp's
  `pokered/constants/map_constants.asm` names.
- Scratch measurement file (`world/zz_gaps_test.go`) was deleted before
  commit.
