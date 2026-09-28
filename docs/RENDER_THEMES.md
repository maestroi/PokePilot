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

## Validation and asset policy

`validateThemePack` rejects incompatible schema versions, invalid IDs, invalid sizing, missing required tiles, malformed paint definitions, malformed asset maps, remote asset URLs, path traversal, unsafe encodings, and unsupported image types. Theme asset references are local-only under `/theme-assets/` and may use PNG or WebP files. SVG, executable content, data URLs, and arbitrary remote origins are intentionally not accepted.

`validateThemeAssetFile` is the canonical boundary for local/community asset files: names must be safe relative paths, only PNG/WebP MIME types are accepted, and a single file is capped at 4 MiB. There is currently no public upload or shared community-theme endpoint. Custom packs are therefore developer/local-install only; any future importer must pass both the manifest and file policy before installation.

Bundled asset directories must contain a machine-readable `provenance.json` with at least a source, license identifier, and an entry for every bundled image. Frontend CI walks every bundled image and every theme asset reference, so adding an unprovenanced image or a reference outside its declared local asset pack fails the test suite.

Gold/Silver-derived art remains explicitly `NOASSERTION`, not “licensed because it is on GitHub.” This policy hardens provenance and ingestion but does not turn those game-derived files into redistributable assets. Deciding whether to gate, replace, or exclude that prototype pack from a public distribution remains the final #1425 distribution-policy item.
