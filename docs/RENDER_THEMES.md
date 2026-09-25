# Spectator render themes

The semantic spectator renderer consumes a viewer-selected **theme pack**. A theme changes presentation only; the ROM, emulator, `RenderState`, and gameplay remain authoritative and unchanged.

## Manifest version 1

Theme packs use a versioned JSON manifest. Bundled themes live under `web/src/shared/themes/` and are installed through the same `RenderThemeRegistry` validation path that future theme sources can use.

```json
{
  "schemaVersion": 1,
  "id": "rompilot-modern",
  "name": "RomPilot Modern",
  "version": 1,
  "description": "Clean high-contrast spectator theme.",
  "tileSize": 36,
  "tiles": {
    "unknown": { "fill": "#293a40", "pattern": "unknown" },
    "path": { "fill": "#d0c493", "pattern": "path" },
    "wall": { "fill": "#526268", "pattern": "wall" },
    "grass": { "fill": "#4d9153", "detail": "#dcffcf59", "pattern": "grass" }
  },
  "objects": {},
  "actors": {
    "player": { "fill": "#f4f7ff", "stroke": "#e43c4f" }
  },
  "animation": {
    "waterPeriodMs": 700,
    "redrawIntervalMs": 100
  },
  "effects": {
    "background": "#142228",
    "vignette": "rgba(0,0,0,.28)",
    "grid": "rgba(255,255,255,.045)",
    "shadow": "rgba(0,0,0,.22)"
  },
  "ui": {
    "accent": "#67e8f9",
    "panel": "rgba(2,6,23,.72)",
    "text": "#cffafe"
  },
  "assets": {
    "tiles": {},
    "characters": {
      "player": "gen1:red"
    },
    "objects": {},
    "effects": {},
    "ui": {},
    "battle": {}
  },
  "battle": {}
}
```

`schemaVersion` versions the manifest format. `version` versions that particular theme. Version 1 supports semantic tile/object paint definitions, actor fallback styling, animation timing, full-scene effects, UI tokens, and namespaced asset references for tiles, characters, objects/buildings, effects, UI, and future battle presentation.

Asset references are only consumed from bundled packs in this slice. Safe ingestion of arbitrary/community files, path validation, provenance, and licensing belong to #1425.

## Required and optional semantics

Every compatible pack must define `unknown`, `path`, and `wall`. Those are the minimum safe surface needed to render an arbitrary overworld without invisible geometry.

`floor`, `grass`, `water`, `tree`, `ledge`, `door`, `warp`, and `sign` are optional. Missing optional tile styles inherit from the default **RomPilot Modern** pack. Actor styles, object styles, animation settings, effects, UI tokens, and asset maps also inherit field-by-field from the default pack.

Unknown semantic tile kinds fall back to the selected theme's resolved `unknown` style.

## Validation and failure behavior

`validateThemePack` rejects incompatible schema versions, invalid IDs, invalid sizing, missing required tiles, malformed paint definitions, and malformed asset maps. Optional omissions return warnings rather than failures because the default theme supplies them.

`RenderThemeRegistry.install` never installs an invalid pack. Resolving an unknown theme ID returns the default theme plus a human-readable diagnostic instead of breaking the renderer.

Bundled manifests are validated at module startup; an invalid bundled pack therefore fails frontend verification rather than shipping silently.

## Viewer selection

The spectator exposes a theme selector while **Modern** rendering is selected. The choice is stored under `pokepilot.spectator.theme` in browser-local storage. It is not stored on the run, sent to the worker, or included in farm state.

That means two viewers can watch the same `RenderState` with different themes at the same time. Switching themes during a live run changes only the Canvas presentation. **Classic** (the emulator framebuffer) is the default renderer mode; viewers opt into Modern explicitly, and that choice is stored under `pokepilot.spectator.renderer`.

Bundled v1 themes:

- **RomPilot Modern** — the default clean spectator presentation.
- **Retro 16-bit** — a chunkier, more saturated alternative using the same semantic state.
- **Tiny Town Pixel** — 16×16 Kenney Tiny Town terrain plus matching Tiny Dungeon interior tiles, drawn at 32 screen pixels with nearest-neighbor scaling. Their bundled images and CC0 source records live under `web/public/theme-assets/`.

## Bundled atlas tiles

`assets.tiles` and `assets.objects` can name a PNG tile in a packed atlas with `/path/to/atlas.png#tile=column,row,size`. Columns and rows are zero-based; `size` is the square tile width in source pixels. The browser loads each atlas once, crops the selected tile, and draws it without smoothing. A plain image URL is also accepted for a single tile. If an image is unavailable, the theme's paint pattern is used.

For nine-slice terrain such as a dirt path, the renderer looks for `path.center` and optional `path.top-left`, `path.top-center`, `path.top-right`, `path.middle-left`, `path.middle-center`, `path.middle-right`, `path.bottom-left`, `path.bottom-center`, and `path.bottom-right`. It chooses a piece from adjacent semantic cells; missing pieces fall back to `path.center`. A producer can also set `TileCell.variant` for a presentation distinction such as `path.paved`.

The Red adapter now marks its known plain ground and forest ground as grass, marks indoor walkable cells as floor, and identifies the overworld's paved tile with a presentation variant. This keeps the theme generic: native Red tile IDs remain in the Red adapter. Building cells still have only portable wall/path information where the adapter cannot identify a facade or roof. The Tiny Town theme draws a generic facade from adjacent wall cells; it cannot yet recreate a specific building's shape or identity.
