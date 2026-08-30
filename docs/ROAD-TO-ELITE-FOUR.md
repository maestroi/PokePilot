# The road from Cerulean City to the Elite Four

Survey of everything that stands between Cerulean City (map `0x03`) and the
Pokémon League (Indigo Plateau `0x09` → lobby `0xAE` → rooms `0xF5`/`0xF6`)
in this ROM. Every claim below was measured against
`roms/pokemon_red.gb` with `skill.TestProbe` / a scratch BFS over
`world.Build` grids, or read from the vendored decomp at `pokered/` (which is
a decomp of *this* ROM — e.g. it already contains the rearranged gyms).
Citations are `pokered/<file>:<line>`.

## The road, leg by leg

Map-level shortest path from `world.BuildGraph` (228 parseable maps, 221
reachable from Cerulean; unreachable: `0B 45 4B 4E AD E7 EF F0`):

```
03 -> 0F -> 0E -> 02 -> 0D -> 01 -> 21 -> 22 -> 09
Cerulean Route4 Route3 Pewter Route2 Viridian Route22 Route23 Indigo
```

| # | Leg | Kind | What actually happens |
|---|-----|------|-----------------------|
| 1 | Cerulean `0x03` —W→ Route 4 `0x0F` | land | walkable, verified by BFS |
| 2 | Route 4 `0x0F` —S→ Route 3 `0x0E` | land | walkable, verified by BFS |
| 3 | Route 3 `0x0E` —W→ Pewter `0x02` | land | walkable, verified by BFS |
| 4 | Pewter `0x02` —S→ Route 2 `0x0D` | land | walkable, verified by BFS |
| 5 | Route 2 `0x0D` —S→ Viridian `0x01` | **phantom edge** | row 22 is a full-width solid wall; the real crossing is through Viridian Forest (see G1) |
| 6 | Viridian `0x01` —W→ Route 22 `0x21` | land | walkable (east edge of Route 22, nearest reachable tile `(39,9)`) |
| 7 | Route 22 `0x21` —N→ Route 23 `0x22` | **phantom edge** | rows 0–1 fully walled; the real exit is gate building `0xC1`, Boulder badge (see G3) |
| 8 | Route 23 `0x22` —N→ Indigo `0x09` | **water + badges** | three full-width water/cliff bands require Surf; seven badge-check sprites (see G4) |

"Phantom edge" = the map header declares a connection (`kind=1` in the
connection list) but the tiles at both ends of that edge are solid, so no
walkable path crosses it. The game's own connection data still lists them
(`pokered/constants/map_constants.asm:43` ROUTE_2 `$0D`, `:63-64`
ROUTE_22/23, `:82-87` the forest gate buildings).

## The gates

### G1 — Route 2's wall (leg 5): physical, no item, cross via Viridian Forest

Route 2 is 20×72 tiles. Row 22 is solid across all 20 columns
(metatiles `$50`/`$3D`, not in the OVERWORLD walkable list). North of it
(rows 0–21): the Pewter side (leg 4 enters here); south of it (rows 23–71):
the Viridian side (leg 5 exits there). BFS confirms neither edge reaches
the other on foot.

The crossing is the forest, through two gate buildings:

- South side: door at Route 2 `(3,43)` → `0x32`
  (VIRIDIAN_FOREST_SOUTH_GATE,
  `pokered/constants/map_constants.asm:86`); `0x32`'s inner door leads to
  the forest's south edge.
- Viridian Forest `0x33` (34×48) is walkable end to end: measured
  **130-step path** from the north-entry area `(2,1)` to the south-edge
  warp `(16,47)`.
- North side: forest north-edge warps `(1,0)/(2,0)` → `0x2F`
  (VIRIDIAN_FOREST_NORTH_GATE,
  `pokered/constants/map_constants.asm:83`); `0x2F`'s inner door leads back
  out to Route 2 `(3,11)`.

No badge, no HM, no script check — just geometry. The agent has already
done this crossing in a live run (it reached Cerulean via Pewter), so it is
proven doable with current verbs; it is not yet expressible in the planner's
map-level graph, which sees `0x0D` as one node.

Also on Route 2, for completeness: Diglett's Cave warp `(12,9)` → `0x2E`,
and the Route 2 Gate (below).

### G2 — Route 2 Gate `0x31` (on leg 5): gives Flash, does not block

Oak's aide inside `0x31` (ROUTE_2_GATE, `pokered/constants/map_constants.asm:85`)
gives HM05 Flash unconditionally:
`pokered/scripts/Route2Gate.asm:11` (`CheckEvent EVENT_GOT_HM05`) and
`:27` (`SetEvent EVENT_GOT_HM05`). The building's doors are at Route 2
`(16,35)` and `(15,39)`; the rock band it sits in (rows 53–56) is walkable
around on either side. Not a gate — but Flash will matter for the Saffron
detour (dark gym), which is unmeasured here.

### G3 — Route 22 Gate `0xC1` (leg 7): Boulder badge

Route 22's north edge is walled (rows 0–1 solid). The only exit is the gate
building: door at Route 22 `(8,5)` → `0xC1`, whose warps lead to Route 23
`(7,139)`/`(8,139)`. Inside, one guard checks the badge:

- `pokered/scripts/Route22Gate.asm:63-64` — `ld a, [wObtainedBadges]` /
  `bit BIT_BOULDERBADGE, a`
- `:66-68` — without it: refusal text + `Route22GateMovePlayerDownScript`
  (the player is pushed back; the door does not open)

BOULDER is granted by Pewter City's gym — `pokered/scripts/PewterGym.asm:65-67`
(`set BIT_BOULDERBADGE`) — which is on the road (leg 3). Badge bits:
`pokered/constants/ram_constants.asm:56-63` (Boulder=0 … Earth=7); the
gym→badge table is `pokered/data/maps/badge_maps.asm` (`MapBadgeFlags`).

Graph caveat: `0xC1`'s own warps all point at `0xFF` ("the map you came
from"), so `world.BuildGraph` resolves it as a dead end — the planner cannot
currently route *through* it. That is a routing gap, not a verb gap.

### G4 — Route 23 (leg 8): Surf + seven badges

Route 23 is 20×144 tiles, tileset PLATEAU (`$23`). Terrain classification
(walkable / water `$14` / other), measured:

- **Rows 81, 92, 101 are full-width non-walkable.** Rows 81 and 92 are
  water on one side and cliff tiles (`$28`/`$0D`/`$03`/`$2E`) on the other;
  row 101 is all `$14` water. PLATEAU's walkable list is only
  `$1b $23 $2c $2d $3b $45` (`pokered/data/tilesets/collision_tile_ids.asm:69-70`,
  identical in the ROM), so none of those rows can be crossed on foot.
- PLATEAU is a water tileset — `pokered/data/tilesets/water_tilesets.asm:11`
  — and the Surf check accepts exactly `$14` water plus shore tiles
  (`pokered/engine/items/item_effects.asm:2829` `IsNextTileShoreOrWater`,
  `cp $14` at `:2844`). So **Surf crosses these bands** (the cliff tiles are
  the banks of the waterfall channel, not obstacles to the swim).
- A walk-only BFS from the gate exit `(7,139)` reaches the west/east edges
  but *not* the north edge — the water bands are what partition the map.
  From the north-edge pocket `(9,1)` (walkable at columns 9–10, rows 0–5) a
  26-step path reaches row 20, so once you can swim, the road to Indigo is
  plain walking.

Seven sprites block the route; five are guards (sprite `0x31`,
`SPRITE_GUARD`) and two are swimmers (`0x22`, `SPRITE_SWIMMER`), standing at
the badge-check points — the two swimmers, and Guard 3 as well, stand in
the water:

| Sprite | Position | Checks | Badge granted by |
|--------|----------|--------|------------------|
| Guard 1 | (0,31) | EARTH | Viridian gym (Giovanni) — on the road |
| Guard 2 | (6,52) | VOLCANO | Cinnabar gym — detour |
| Swimmer 1 | (4,81) | MARSH | Saffron gym — detour |
| Swimmer 2 | (7,92) | SOUL | Fuchsia gym — detour |
| Guard 3 | (8,101) | RAINBOW | Celadon gym — detour |
| Guard 4 | (4,115) | THUNDER | Vermilion gym — detour |
| Guard 5 | (4,132) | CASCADE | Cerulean gym — on the road (you're starting there) |

Check logic: `pokered/scripts/Route23.asm:153-193` (one text script per
sprite; the `EventFlagBit` lines at `:155,161,167,173,179,185,191` select the
badge), then `Route23CheckForBadgeScript` at `:195-219`:
`FLAG_TEST` on `wObtainedBadges` (`:201-202`); no badge → refusal text +
`Route23MovePlayerDownScript` (pushed back), badge → sets the
`EVENT_PASSED_*BADGE_CHECK` flag and lets you through.

So leg 8 needs **all eight badges** (Boulder at G3 plus these seven) and a
working Surf. Gym→badge mapping as in this ROM:
`pokered/data/maps/badge_maps.asm` — Pewter/Boulder, Cerulean/Cascade,
Vermilion/Thunder, Celadon/Rainbow, Fuchsia/Soul, Saffron/Marsh,
Cinnabar/Volcano, Viridian/Earth.

### G5 — Indigo Plateau `0x09`: no gate at the door

`pokered/scripts/IndigoPlateau.asm` is five lines (text pointers only) — no
`CheckEvent`, no badge check, no guard sprites. The entrance warps are
`(9,5)`/`(10,5)` → `0xAE` (Pokémon League lobby, 16×12), which chains to
`0xF5` → `0xF6`. The final "gate" is the Elite Four battle itself, which
nothing in the current objective vocabulary can start.

## Where Surf comes from in this ROM

The Warden's House is map `0x9B` (WARDENS_HOUSE,
`pokered/constants/map_constants.asm:245`, LAB tileset). In this ROM it is
**inside Fuchsia City**: the only warp into it is Fuchsia `0x07` `(27,27)`
→ `0x9B` (full-ROM warp scan; nothing else points at it). The Warden gives
HM04 Surf once you have dealt with the GOLD_TEETH item:
`pokered/scripts/WardensHouse.asm:14` (`CheckEvent EVENT_GOT_HM04`) and
`:47` (`SetEvent EVENT_GOT_HM04`). GOLD_TEETH is a field pickup in Safari
Zone West (`pokered/scripts/SafariZoneWest.asm:9`).

The route Cerulean → Fuchsia / Safari Zone was **not** surveyed (out of
scope for this task); whether it needs Surf itself is an open question that
the first badge-tour slice must answer. (Cinnabar in particular is
water-adjacent in stock geography.)

## Q3 — can the graph route Cerulean → Indigo at all, ignoring gates?

**Yes, at map level: 8 legs.** The single most useful number: of those 8
legs, **2 are phantom connection edges** (legs 5 and 7 — the header claims
adjacency, the tiles are walled) and **1 is water-gated** (leg 8). Legs 1–4
and 6 are verified on-foot paths. The planner's `Travel`/`GoTo` can execute
legs 1–4 and 6 today; leg 5 was solved by exploration in a live run (forest
detour) but is invisible to the map-level graph, which treats `0x0D` as one
node; leg 7 needs the dead-end building `0xC1` to become passable in the
graph; leg 8 needs Surf.

## Q4 — verb gaps

Current kinds (`agent/objective.go:17-27`): GoTo, Talk, Starter, Errand,
Train, Heal, Gym, Catch, Buy, Pickup, UseItem.

1. **Surf / HM field-use** — the biggest wall. `KindUseItem` uses a bag item
   on a *party member*; there is no verb for using an HM on the world or for
   swimming across water. Without it, leg 8 (and probably the Cinnabar
   detour) are impossible.
2. **Elite Four battle** — `KindGym` fights "the leader of whichever gym the
   player is in"; there is no verb for the League's four-plus-one sequence
   (or for any non-gym trainer).
3. **Routing gaps, not verbs**: phantom edges (Route 2 split at row 22;
   Route 22's walled north edge) and the dead-end gate `0xC1`. The graph
   needs sub-node routing or warp-through-building handling, or the planner
   needs to learn the detours the way it already learned the forest
   crossing.
4. **Badge detours** — five off-road gyms (Cinnabar, Saffron, Fuchsia,
   Celadon, Vermilion). `KindGym` exists; the travel to them is unmeasured,
   and at least one (Cinnabar) may itself require Surf. Flash (G2) may be
   needed for the Saffron gym's darkness — unmeasured.

## Q5 — how many slices

Seven, in dependency order:

| Slice | Contents | Notes |
|-------|----------|-------|
| S10-1 | Surf verb: use HM04 on water, swim to a target tile/shore; postcondition from RAM (position + not-in-battle) | Foundational; unblocks leg 8 and Cinnabar. Includes the acquisition chain as planning (Safari Zone West GOLD_TEETH `KindPickup` + Fuchsia Warden `KindTalk` — no new verb needed for the hand-off itself) |
| S10-2 | Route 22 gate: Boulder via Pewter `KindGym` + make `0xC1` passable in the graph (or planner knowledge of the door at `(8,5)`) | Small; mostly a graph fix |
| S10-3 | Badge tour A: Fuchsia (Soul) + Celadon (Rainbow) | Adjacent cities; answers "does the detour need Surf?" |
| S10-4 | Badge tour B: Vermilion (Thunder) + Saffron (Marsh) | Saffron may need Flash (G2) — verify, don't assume |
| S10-5 | Badge tour C: Cinnabar (Volcano) | Surf-dependent; last badge |
| S10-6 | Route 23 traversal: swim the three bands past the seven checks | Pure execution once badges + Surf exist; good fixture target |
| S10-7 | Elite Four battle verb + Indigo finale | New kind; ends the journey |

S10-3/4/5 could merge into one "badge tour" slice if each gym proves to be a
short walk from the last; S10-6 and S10-7 could merge if the League battle
verb turns out small. Floor is five, ceiling eight.

## Method notes

- Grids came from `world.Build` (per-tileset walkable lists); the PLATEAU
  list in the ROM matches the decomp byte-for-byte, and the block data for
  Route 2 / Route 22 / Route 23 / Indigo Plateau / Viridian Forest matches
  the vendored `pokered/maps/*.blk` files.
- One dead end worth recording: map `0x0B` (UNUSED_MAP_0B,
  `pokered/constants/map_constants.asm:39`) contains warp entries pointing at
  Indigo, but its header data is invalid (block address below `0x4000`) and
  it does not parse — treat as dead data, not a second route in.
- Scratch measurement file used for this survey: `agent/zz_road_test.go`
  (deleted before commit; output was written to a temp file, not context).
