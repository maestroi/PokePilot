# Live map view — design

**Date:** 2026-09-01  
**Status:** proposed, not implemented

Add a bird's-eye map next to the Game Boy LCD in pokeui's watch pane: the
full current map, the player's tile, live sprite blockers, and optionally a
short movement trail. The LCD shows what the game draws; this shows where the
run is on the map — the thing a 160×144 viewport cannot.

This does not change planner behaviour, pathfinding, or the farm lease
contract beyond one optional heartbeat field. `emu/`, `skill/`, and `agent/`
are untouched except for a small heartbeat enrichment in `cmd/pokepilot`.

---

## 1. Problem

pokeui already proxies the runner's live screen (`GET /frame`) and shows
map id plus `(x, y)` on each card. That is enough to know *which* map, not
*where on it*.

Gen 1 maps are much larger than the viewport. Route 2 is 20×72 game tiles;
Viridian Forest is 34×48. On the LCD you see a few tiles of grass, a sprite,
maybe a ledge — not the loop the agent is tracing, the warp it is missing, or
the NPC occupying a choke point three screens away.

`skill/probe_test.go` already answers geometry questions with a small ASCII
window (`@`, `#`, `.`, `x`). Operators today run that from a terminal. The
watch pane is the natural home for the same information, rendered for eyes
instead of an agent context budget.

---

## 2. Goal

When an operator opens a live run in pokeui:

1. **LCD** — unchanged; the faithful 160×144 frame.
2. **Map panel** — the full current map, updating every dashboard poll:
   - player position on the walkability grid;
   - live map objects (sprite slots 1–15) as blockers;
   - optional faint trail of recent `(x, y)` samples;
   - warps and map edges marked when in semantic mode.

Two render tiers, one wire format:

| Tier | Look | Source |
|------|------|--------|
| **Semantic** (v1) | Coloured grid: grass, wall, warp, water-ish | `world.Build` walkability + ROM header warps |
| **Tile** (v2) | Actual map blocks from the ROM tileset | `rom.Blocks` + tileset gfx, precomputed |

Both tiers share the same live overlay (player dot, sprites, trail). Semantic
ships first because it needs no gfx decode and matches what pathfinding uses.

---

## 3. Why not "RAM of all maps"

WRAM describes **one map at a time**:

- `wCurMap`, `wXCoord`, `wYCoord` — player
- `wSpriteStateData2` / `wSpritePlayerStateData1` — live objects on the
  **current** map only (`red/state/sprite.go`)
- `wTileMap` — the 20×18 LCD background window, not a world map

Collision geometry, block layout, warps, and connections live in the **ROM**
(`red/rom`, `world/`). A save state is one moment on one map.

So: **ROM (or precomputed ROM exports) for the static picture; RAM (via
heartbeat) for the moving dots.** There is no design that reads "all maps"
out of RAM simultaneously — and none is needed for this feature.

---

## 4. Architecture

```
browser  --poll-->  pokeui
                       |  GET /v1/dashboard  (map, x, y, sprites, trail)
                       |  GET /frame?run=    (LCD PNG, unchanged)
                       |  GET /assets/maps/{id}.json   (semantic grid, static)
                       |  GET /assets/maps/{id}.png    (tile render, static, v2)
                       v
                     wall  (pass-through only; no ROM)
                       ^
                       |  heartbeat carries Sprites + Trail
                     runner (cmd/pokepilot — has ROM + RAM)
```

**ROM stays on the runner** (or in a build-time generator). The wall remains
ROM-free so `env -u POKEMON_RED_ROM go test ./cmd/pokewall/...` keeps working.

Static map assets are baked once from the vendored ROM and embedded in pokeui
(or served from a `maps/` directory built by `go generate`). Live positions
ride the existing heartbeat → dashboard path.

---

## 5. Wire format

### 5.1 Heartbeat extension (`farm/spec.go`)

Add optional fields to `Heartbeat`:

```go
// MapSprite is one live map object on the runner's current map.
type MapSprite struct {
    X         uint8 `json:"x"`
    Y         uint8 `json:"y"`
    PictureID uint8 `json:"picture_id,omitempty"` // for tooltips, not rendering
}

// Heartbeat — new fields:
Sprites []MapSprite `json:"sprites,omitempty"` // slots 1..15, current map only
Trail   [][2]uint8  `json:"trail,omitempty"`     // recent (x,y), oldest first
```

- **Sprites:** decoded with `state.DecodeSprites(m)` each tick. Same rules as
  pathfinding: ephemeral, never cached as learned geometry across heartbeats.
- **Trail:** runner keeps a fixed-length ring (default 64 samples, one per
  heartbeat ~1 Hz ≈ one minute). Cleared on map change. Omit when empty.
- **Nil/empty on scripted runs** is fine; the map panel still shows the player
  dot from `Map`/`X`/`Y`.

Old runners → new wall: fields absent, panel renders player only.  
New runners → old wall: extra JSON ignored.

Wall copies `Sprites` and `Trail` into `Tile` / `tileRow` / dashboard JSON.
Like `Raw`, they are live-only — not persisted across wall restart.

### 5.2 Static map asset (semantic, v1)

One JSON file per parseable map id, generated at build time:

```json
{
  "id": 12,
  "width": 20,
  "height": 72,
  "cells": "....................#...", 
  "warps": [{"x": 11, "y": 0, "dest": 47}],
  "connections": ["north"]
}
```

- `cells`: row-major string, one rune per game tile (`world.Grid` coordinates).
  Proposed runes:
  - `.` walkable land
  - `#` solid
  - `g` tall grass (from tileset grass tile + block data where cheap)
  - `~` water/surf (collision-non-walkable but visually distinct, optional)
  - `W` warp tile (from ROM header, not collision alone)
- Dimensions match `world.Build` output exactly.
- File size: largest map ≈ 34×48 = 1632 bytes of cell data — trivial.

Generator: `go run ./cmd/mapassets` (new) or a `//go:generate` in `world/`,
guarded by `POKEMON_RED_ROM`. Committed output under
`cmd/pokeui/ui/maps/` so pokeui and CI need no ROM.

### 5.3 Static map asset (tile, v2)

One PNG per map plus a small manifest:

- Decode each map's block list (`rom.Blocks`) through the tileset's gfx
  (`pokered` tileset table at `03:47BE`, same table `world/grid.go` uses).
- Each **game tile** is 16×16 px (a 2×2 block step = one 4×4-tile block in
  ROM terms, composited down to the walkability cell the agent uses).
- PNG dimensions: `width×16` by `height×16` pixels.
- Store `{id}.png` next to `{id}.json`; pokeui picks PNG when present, else
  falls back to semantic canvas.

This is cosmetic. Pathfinding and the semantic grid remain authoritative.

---

## 6. Runner changes (`cmd/pokepilot`)

In the existing heartbeat goroutine (same place `Stats` and `Question` are
snapshotted):

1. Read RAM via the emulator handle already held for observation.
2. `sprites := state.DecodeSprites(mem)`.
3. Append `(obs.X, obs.Y)` to a per-run ring buffer; clear buffer when
   `obs.Map` changes.
4. Push `Sprites` and `Trail` into `heartbeatSnap` alongside `Map`/`X`/`Y`.

No new HTTP routes on the runner. No ROM work at heartbeat time for v1.

---

## 7. pokeui layout

Extend the `#watch` detail pane (wide layout already has LCD + side column):

```
┌─ watch: run-id ─────────────────────────────────────────────┐
│  ┌─────────────┐   ┌─────────────────────────────────────┐ │
│  │  LCD 160×144 │   │  Map (canvas, scroll if needed)     │ │
│  │  (existing)  │   │  · full map, player dot             │ │
│  └─────────────┘   │  · sprite markers                   │ │
│                    │  · trail                            │ │
│  meta / Plan / …   │  legend: # wall · g grass · @ you   │ │
│                    └─────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────┘
```

**Behaviour:**

- Map panel visible only for **running** (or recently running) tiles with a
  known `map` id and a loaded asset. Missing asset → show map id + coordinates
  only (today's behaviour).
- Canvas scales to fit (`max-width: 100%`, integer scale factor so pixels
  stay crisp in tile mode).
- If map is taller than the pane, allow vertical scroll; optionally auto-pan
  to keep the player dot in view (nice-to-have, not v1).
- Hover a sprite marker → tooltip with picture id / slot (debugging).
- Trail: semi-transparent dots or a polyline, oldest faint.

**Live cards** (grid view): no map canvas — too small. Optional future: a
16×16 thumbnail with a dot. Out of scope for v1.

Styling: match existing Game Boy palette (`--lcd`, `--lcd-dark`, `--panel`).
Semantic mode uses flat fills, not a second LCD bezel.

---

## 8. Rendering (browser)

All client-side; no canvas library.

### 8.1 Semantic (v1)

1. Fetch `/maps/{id}.json` once per map id (cache in a `MapAssetCache` in
   `ui.js`).
2. `<canvas>`: one rect per cell, 4×4 or 6×6 CSS pixels per tile.
3. Draw layers bottom → top:
   - cell colour from rune
   - trail segments
   - sprite markers (distinct colour/shape from player)
   - player (bright `@` or filled circle)
4. Warp tiles: ring or `W` glyph.

Same information as `probeGrid`, different medium.

### 8.2 Tile (v2)

1. Fetch `{id}.png` (cache aggressively — immutable content).
2. Draw PNG at integer scale.
3. Overlay trail, sprites, player as in semantic mode (vector on top of
   pixels).

Optional polish: extract the tileset once into a shared atlas and compose in
the browser from `{id}.json` block indices instead of per-map PNGs — smaller
repo, more JS. Per-map PNGs are simpler to generate and debug; prefer PNGs
unless repo size becomes a problem (~200 maps × ~50 KB ≈ 10 MB, acceptable).

---

## 9. Build-time asset generator

New command: `cmd/mapassets` (name tentative).

```
POKEMON_RED_ROM=roms/pokemon_red.gb go run ./cmd/mapassets \
  -o cmd/pokeui/ui/maps
```

For each parseable map id (same loop as `world.BuildGraph`):

1. `rom.ParseMap`, `world.Build` → semantic runes + dimensions.
2. Write `{id:02x}.json`.
3. (v2) Render PNG from blocks + tileset gfx.

Commit generated files. CI verifies they are fresh:

```
go run ./cmd/mapassets -o /tmp/maps && diff -r cmd/pokeui/ui/maps /tmp/maps
```

(or a `-check` flag that exits non-zero on drift).

pokeui embeds `ui/maps/*` via `//go:embed` and serves at `GET /maps/...`.

---

## 10. Testing

| Layer | What |
|-------|------|
| `farm` | Heartbeat JSON round-trip with `sprites` / `trail` set and unset |
| `cmd/pokepilot` | Heartbeat snap includes decoded sprites; trail resets on map change |
| `cmd/pokewall` | Dashboard JSON carries new fields; not persisted in state file |
| `cmd/pokeui` | Source-grep: canvas element, map fetch, render hooks; console test for embed route |
| `cmd/mapassets` | Golden file: one known map (e.g. Pallet Town 0x00) JSON matches committed asset |
| `world` / `red/rom` | Unchanged; generator reuses existing parsers |

No journey tests. No emulator in pokewall tests.

Manual smoke: wall + pokeui locally, queue a run that walks around Pallet
Town, open watch pane — dot moves, trail grows, LCD and map agree on position.

---

## 11. Verification

```sh
go test ./...
POKEMON_RED_ROM=roms/pokemon_red.gb go run ./cmd/mapassets -check
# manual: pokeui + wall, watch a walking run
```

Farm infra still passes without ROM:

```sh
env -u POKEMON_RED_ROM go test ./cmd/pokewall/... ./cmd/pokeui/... ./farm/...
```

---

## 12. Incremental delivery

| Step | Delivers |
|------|----------|
| **A** | `mapassets` semantic JSON + embed in pokeui |
| **B** | pokeui canvas, player dot only (from existing dashboard `map/x/y`) |
| **C** | Heartbeat `sprites` + `trail`; overlay on canvas |
| **D** | Auto-scroll / warp legend / grass distinction polish |
| **E** | `mapassets` tile PNGs (v2 cosmetic) |

Each step is shippable. B is useful before C (position alone beats LCD-only).

---

## 13. Out of scope

- **World graph view** (all maps as a node graph) — complementary feature,
  separate doc.
- **Rendering collision bytes from save states in the browser** — the 2026-08-30
  issue-handoff design explicitly deferred this; static precomputed assets
  replace that need.
- **Uploading or serving the ROM** from pokeui or the wall.
- **Caching sprite positions as learned geometry** — violates AGENTS.md; every
  heartbeat rebuilds from RAM.
- **Map panel on every live card** — watch pane only in v1.
- **Reading full collision grids into agent/LLM context** — unchanged; agents
  keep using `TestProbe` and code.

---

## 14. Open questions

1. **Trail length** — 64 samples default; expose as runner flag if operators
   want longer history?
2. **Grass / water runes** — worth the generator complexity in v1, or ship
   `.` / `#` only and add terrain classes in step D?
3. **Finished runs** — keep last map + trail in dashboard until cleared, or
   hide map panel when status ≠ running?
4. **Tile mode priority** — if both JSON and PNG exist, always prefer PNG,
   or operator toggle?

---

## 15. References

- `skill/probe_test.go` — `probeGrid`, the semantic prototype
- `world/grid.go` — walkability grid construction
- `red/state/sprite.go` — live object decode
- `farm/spec.go` — heartbeat wire types
- `cmd/pokeui/ui/index.html` — `#watch` layout hook
- `docs/archive/2026-08-30-pokefarm-issue-handoff-design.md` § Out of scope
  (prior deferral of collision rendering in browser)
