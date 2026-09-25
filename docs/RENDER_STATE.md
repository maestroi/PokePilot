# Semantic RenderState protocol

`renderstate.RenderState` is the presentation contract between PokePilot game adapters and spectator renderers. It is deliberately independent of the framebuffer, RAM addresses, ROM table layouts, and frontend implementation.

The same state shape is intended for:

- live public spectator rendering;
- the operator live view;
- recorded replay playback.

Transport is not part of the contract. HTTP polling, SSE/WebSocket streaming, and replay files can all carry the same JSON representation.

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
