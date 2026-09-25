# Semantic RenderState protocol

`renderstate.RenderState` is the presentation contract between PokePilot game adapters and spectator renderers. It is deliberately independent of the framebuffer, RAM addresses, ROM table layouts, and frontend implementation.

The same state shape is intended for:

- live public spectator rendering;
- the operator live view;
- recorded replay playback.

Transport is not part of the contract. HTTP polling, SSE/WebSocket streaming, and replay files can all carry the same JSON representation.

The live public spectator and private operator live view both consume the same validated `/render-state` feed and mount the shared `ModernSceneRenderer` component. Each viewer can select a theme locally, while unsupported scenes, unavailable semantic state, and explicit Classic mode fall back to the authoritative framebuffer. Replay remains a transport concern: recorded playback should feed reconstructable semantic states into this same renderer rather than create a separate game-view implementation.

## Versioning

`schema_version` is currently `1`.

Within one schema version, producers may add optional fields, new capabilities, and new string values for scenes, entity kinds, tile semantics, movement kinds, effects, and similar open vocabularies. Consumers must ignore JSON fields they do not understand and fall back safely for unknown semantic values.

A schema version bump is reserved for a breaking wire change. `renderstate.ReadJSON` rejects an unsupported schema version rather than silently interpreting it as a different contract.

## Ownership

Generic `renderstate` owns portable presentation concepts:

- emulator frame/cycle and optional capture time;
- scene identity;
- semantic map identity and camera;
- player/entity positions, facing, and movement;
- semantic tile layers;
- optional dialogue, menu, battle, transition, and transient-effect state;
- producer capabilities.

Game adapters own the facts needed to populate those concepts. Native map IDs, sprite picture IDs, RAM addresses, ROM tile/block numbers, event flags, and generation-specific structures do not belong on the RenderState wire.

The baseline `renderstate.FromProfile` asks any `game.GameProfile` for its portable observation and converts it without frontend-specific logic; `FromProfileObservation` is available when that observation has already been decoded. It intentionally does not invent missing world details. Rich map tiles, stable NPC identity, movement segments, camera state, and other Red/Blue/Yellow extraction belong in adapter-owned producers (see #1419).

## Coordinates and rendering

World positions and semantic map dimensions use tile coordinates, not pixels. Themes choose their own pixel scale. `CameraState` uses world tile units for the same reason.

`MovementState` records authoritative movement endpoints/progress when known. A renderer may interpolate for presentation, but interpolated state must never feed back into gameplay.

## Capabilities and fallbacks

Optional sections are advertised through `capabilities`. Consumers should feature-detect those capabilities rather than branch on `game.id`.

Examples:

- `map` and `player` are available from the current profile observation baseline;
- `layers` and `entities` indicate semantic world extraction;
- `dialogue`, `menu`, and `battle` indicate decoded scene details;
- `transition` and `effects` indicate richer animation/event support.

Unknown semantic tile/entity/effect values are valid. Renderers should use a neutral fallback asset or omit the unsupported enhancement rather than failing the whole scene.


## Pokémon Red producer

`red/renderstate.Producer` is the first game-owned implementation of the rich world surfaces. It accepts the exact supported Red ROM revision and reuses `red/rom` plus `world` for deterministic map decoding.

Static reconstruction uses ROM-owned map headers, blocks, collision/field tiles, warps, and signs. The adapter translates Red's native tile meanings into semantic terrain such as path, wall, water, grass, tree, and warp; raw tile/block IDs never leave the adapter.

Live snapshots additionally read the current `wOverworldMap` block buffer so script-driven `ReplaceTileBlock` changes are reflected rather than overwritten by static ROM geometry. Player movement and visible entity positions/facing come from the same RAM snapshot. Live object slots are joined to their static map objects only to derive stable semantic identity/kind; their observed coordinates remain ephemeral and are never persisted as map geometry.

Sprite picture IDs are translated to semantic appearance keys such as `professor_oak`, `youngster`, or `poke_ball`. Unknown picture IDs become `unknown`. Unknown terrain that has no Red-specific meaning still degrades to the portable walkable `path` or blocked `wall` fallback.

This package is intentionally Red-owned. Blue can share the Gen-I extraction mechanics through its adapter boundary where appropriate, while Yellow and later generations can provide their own native mappings and still emit the same `renderstate.RenderState` contract.


## Progressive presentation scenes

Pokémon Red enriches the baseline scene model with presentation-only dialogue, menu, and battle state. These surfaces are decoded from the same coherent RAM snapshot as the overworld:

- dialogue text comes from the live Gen-I tilemap decoder;
- a menu is advertised only when the ROM-published cursor glyph is actually drawn, avoiding stale menu RAM;
- battle actors come from the existing Red battle decoder and expose semantic species identity, level, HP/max HP, and status;
- the active player's battle moves expose semantic move identity/name, current PP, ROM-derived max PP when available, and disabled state.

Battle snapshots deliberately do not depend on overworld block geometry. Once the battle engine owns the screen, a temporary or stale overworld map buffer must not make the semantic feed disappear.

These fields are **presentation state, not controller state**. The frontend may choose layout, colors, assets, transitions, and animation, but it must not reimplement damage, RNG, legal-action rules, menu selection, or any other gameplay mechanic. Unsupported sub-scenes remain valid reasons to display the authoritative framebuffer as a compatibility scene.

Schema v1 remains additive: battle actor appearance/level and battle move details are optional fields, so older consumers continue to ignore what they do not understand.


## Spectator animation clock

The browser renderer owns a presentation-only animation clock that is intentionally decoupled from emulator speed. Sparse authoritative snapshots are ingested with their spectator arrival time and sampled at display-refresh time; the clock never mutates or replaces `RenderState`.

The clock follows these rules:

- interpolation duration is based on spectator update cadence, not the number of emulator frames advanced, so 1x, 10x, and 20x runs share the same visual timing budget;
- player and stable-identity entity positions are interpolated only for presentation, with a bounded catch-up window;
- the camera follows a separately smoothed fractional focus, so crossing tile boundaries does not move the viewport in whole-tile jumps;
- repeated source frames are treated as pause/stall heartbeats and do not queue movement;
- a long gap/reconnect snaps to the newest authoritative state instead of replaying stale motion;
- map changes, frame rewinds, and large same-map position jumps are explicit discontinuities and snap rather than masquerading as walking;
- authoritative `MovementState.progress`, when available, is used as the starting semantic position rather than guessed from wall-clock time.

`clock.frame` and `clock.captured_at_unix_ms` remain attached to presentation samples so replay implementations can drive the same deterministic ingest/sample API with recorded timing metadata. The renderer clock has no gameplay output path.
