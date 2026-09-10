# Replay playback speed — Design

**Date:** 2026-09-10
**Status:** approved

## Problem

Finished-run recordings are long. The public spectator player already exposes
1× / 2× / 4× via `video.playbackRate`. The operator Game bay does not: it
relies on native HTML5 controls, where speed is easy to miss and usually
caps at 2×.

## Goal

Both surfaces can play a generated replay at 1×, 2×, 4×, 8×, or 16×.
The last chosen rate is remembered. Live frames stay real-time.

## Decisions

- **Client-side only.** Generated MP4s are silent (`-an`). Speed is
  `HTMLMediaElement.playbackRate`. No re-encode, no pokereplay or wall
  change, no `.gbrun` format change.
- **Same presets on both surfaces.** 1×, 2×, 4×, 8×, 16×. Default 1×.
- **Explicit Speed `<select>`.** Do not depend on the browser's hidden
  native speed menu. Keep native play/seek/fullscreen; do not add a
  duplicate range input (existing console invariant).
- **Remember the last rate** in `localStorage` under
  `pokepilot.replayPlaybackRate`. Invalid or missing values fall back to 1×.
  Private-mode `localStorage` failures are ignored.
- **Show the control only while a replay video is visible.** Live LCD
  frames and generate/error chrome do not show it.
- **Apply the rate on every `canplay`.** Some browsers reset `playbackRate`
  when a new `src` loads.

## Surfaces

1. **Public spectator** (`cmd/pokeui/ui/watch.html`, `watch.js`) — extend
   the existing overlay select.
2. **Operator Game bay** (`inspector.js` mounts into `#detail-game-media`) —
   add the same overlay on the Game media host.

## Out of scope

- Speeding up live emulator frames
- Keyboard shortcuts
- Per-run speed (preference is global to the browser)
- Audio pitch handling (replays have no audio)
