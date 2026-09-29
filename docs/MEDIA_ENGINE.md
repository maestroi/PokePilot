# Media engine package boundaries

PokePilot media generation is split between reusable rendering packages and the
`cmd/pokereplay` worker/API process.

The dependency direction is intentionally one-way:

```text
run.gbrun + RenderState/ReplayTimeline + MediaTimeline + raw/live frame
                              |
                              v
                      cmd/pokereplay
                 HTTP / storage / worker lifecycle
                      |             |
                      v             v
             media/compositor   media/segment
                      \             /
                       \           /
                         media/encode
                  |
                  v
          ffmpeg / encoder process
                  |
                  v
          derived media artifact
```

## Packages

### `media/encode`

Owns the bounded subprocess execution contract used by reusable media code.
`encode.Runner` is injectable, so compositor/assembly behavior can be tested
without an HTTP server or a real encoder process. `ExecRunner` is the
production implementation and retains only a bounded tail of process output.

This package does not know about runs, S3, replay jobs, or HTTP.

### `media/compositor`

Owns presentation composition. The classic broadcast implementation accepts a
`compositor.Scene` containing a raw video plus `farm.MediaTimeline` and writes
the composed destination. The same package also renders individual live frames
with the same timeline plan and drawing primitives.

The package also owns the deterministic headless semantic renderer. It consumes
the existing versioned `renderstate.RenderState` / `ReplayTimeline` contract,
uses an explicit presentation timestamp and viewport, and supports the same
overworld, dialogue, menu, and battle scene classes used by the browser
`ModernSceneRenderer`. Unsupported or transient semantic states fall back to
the authoritative Classic framebuffer on that frame.

The public headless renderer embeds the exact Tiny Town manifest and CC0 atlas
files already used by the browser public theme. Its embed boundary contains no
local-only Gold/Silver assets, so a media worker cannot accidentally package or
load them. Theme/schema/headless renderer versions participate in the semantic
derived-artifact identity.

The compositor does not fetch run state, artifacts, credentials, or media job
state.

### `media/segment`

Owns deterministic assembly of already-rendered media segments. It supports
local files and presigned remote inputs and delegates process execution through
`media/encode`. Signed input URLs are redacted from surfaced encoder errors.

Segment planning that depends on the GomeBoy recording format currently remains
inside `cmd/pokereplay`; only the reusable assembly operation lives here.

## `cmd/pokereplay` responsibilities

The replay binary remains the composition root. It owns:

- HTTP endpoints and request validation;
- pokewall/S3 access and artifact lookup;
- durable render-job claiming, progress, recovery, and worker lifecycle;
- replay-specific GomeBoy reconstruction and segment planning;
- render admission/resource limits;
- wiring reusable compositor and segment implementations together;
- choosing broadcast, semantic, or raw presentation mode while keeping source
  gameplay and viewer/theme presentation independent.

Reusable media packages must not import `cmd/pokereplay` or depend on
`replayServer`.

## Extension rule

Highlights, edited episodes, publishing, and live output should call the media
packages directly rather than invoking pokereplay HTTP endpoints internally.
Server endpoints may submit/observe work, but presentation and encoding logic
belongs below the process boundary.

This keeps source gameplay authoritative and allows historical runs to be
re-rendered with new presentation implementations without coupling those
implementations to worker state.


## Replay presentation modes

The replay API keeps `broadcast` as the compatibility default. `semantic`
selects the headless RenderState renderer and stores its output under a
renderer/theme-versioned cache key. `raw` preserves the legacy game-only
framebuffer path for debugging.

Semantic mode reuses the existing bounded/resumable segment job model. During
the GomeBoy replay callback the worker has both the authoritative framebuffer
and a semantic observation. Supported states render through the public Tiny
Town headless renderer; unsupported scenes, transient semantic decode failures,
and games without a rich semantic adapter use the framebuffer for that frame
instead of invalidating the replay.
