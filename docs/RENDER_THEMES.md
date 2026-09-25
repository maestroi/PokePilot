# Spectator render themes

The semantic spectator renderer consumes a viewer-selected **theme pack**. A theme changes presentation only; the ROM, emulator, `RenderState`, and gameplay remain authoritative and unchanged.

## Bundled themes

The default presentation is **Gold / Silver** (`pokegold-gen2`). It renders Pokémon Red's semantic state with selected Pokémon Gold/Silver Kanto tiles, overworld sprites, Gen-II palettes, and Gold battle sprites. **Tiny Town Pixel** remains as the CC0 alternative. **Classic** is not a theme pack: it is the original emulator framebuffer, and the spectator opens in it by default until a viewer opts into the semantic renderer (stored under `pokepilot.spectator.renderer`).

The old `rompilot-modern` and `retro-16` packs are no longer bundled.

## Manifest version 1

Theme packs live under `web/src/shared/themes/` and are installed through `RenderThemeRegistry`. Every compatible pack defines `unknown`, `path`, and `wall`; optional `floor`, `grass`, `water`, `tree`, `ledge`, `door`, `warp`, and `sign` styles inherit from the default pack when omitted.

Asset namespaces cover tiles, characters, objects, effects, UI, and battle presentation. A tile reference can point at a full PNG or an atlas crop:

```text
/theme-assets/pokegold-gen2/kanto.png?palette=bg-green&repeat=2#tile=12,2,8
```

The fragment identifies an 8×8 source tile. `palette` selects an indexed Gen-II palette at render time and `repeat=2` repeats that source tile across the 16×16 semantic field cell. Nearest-neighbor rendering stays enabled.

## Semantic mapping

The theme never assumes Red tile IDs equal Gold/Silver tile IDs. The Red adapter owns native decoding and emits portable meanings such as `grass`, `path.paved`, `tree`, `water`, `floor`, `wall`, and semantic actor appearances. The Gold/Silver theme maps those meanings onto Gen-II graphics.

This keeps the architecture:

```text
Pokémon Red ROM -> Red adapter -> RenderState -> Gold/Silver theme
```

and lets the same theme machinery work for future game adapters.

## Viewer selection and fallback

Theme choice is viewer-local. Spectator storage uses `pokepilot.spectator.theme`; operator storage uses `pokepilot.operator.theme`. Changing the theme does not mutate or restart a run.

Unsupported or temporarily unavailable semantic scenes fall back to **Classic** without changing the viewer's selected renderer preference.

## Gold / Silver provenance

Selected graphics under `web/public/theme-assets/pokegold-gen2/` are copied from `pret/pokegold`, pinned to upstream commit `0f087a51e36cbd38f33e5055754614578246ceff`. The exact upstream path and blob SHA for every copied file are recorded in `provenance.json`.

Those graphics are game-derived. PokePilot records their license as **`NOASSERTION`**: availability in the disassembly repository is not treated as a separate artwork redistribution grant. They are bundled here for the project's current non-commercial prototype use, with provenance kept explicit so they can be replaced or gated later without confusing them with CC0/original assets.

The Tiny Town/Tiny Dungeon files retain their existing CC0 provenance records.

## Validation

`validateThemePack` rejects incompatible schema versions, invalid IDs, invalid sizing, missing required tiles, malformed paint definitions, and malformed asset maps. `RenderThemeRegistry.install` never installs an invalid pack. Unknown theme IDs resolve to the default Gold/Silver theme with a diagnostic instead of breaking rendering.

Safe ingestion of arbitrary/community files, path validation, upload policy, and broader provenance enforcement remain part of #1425.
