# Run Inspector, artifacts, and deterministic replay

PokePilot keeps run-debugging ownership inside PokePilot. Agent Orchestrator is
an optional consumer through APIs/MCP; it does not own the PokéFarm run catalog,
S3 object keys, or replay cache.

## Ownership

- **pokewall** owns run lifecycle state and durable finish-dump metadata.
- **S3/RustFS** owns large artifact bytes such as `run.gbrun` and derived MP4s.
- **pokereplay** reads artifact references from pokewall, reads/writes S3, and
  has read-only access to the ROM needed by GomeBoy for deterministic replay.
- **pokeui** is the private same-origin relay and Run Inspector UI.
- **Agent Orchestrator** may receive a PokePilot run/issue reference, then use
  PokePilot HTTP/MCP reads when it needs evidence. It never queries a PokePilot
  database or S3 bucket directly.

No new PostgreSQL dependency is introduced by this slice. The finish dumps that
pokewall already owns are the initial artifact index.

## Operator HTTP API

Pokewall adds read-only inspection routes alongside the existing wall protocol:

- `GET /v1/runs/{id}` — one live/finished run plus compact finish metadata.
- `GET /v1/runs/{id}/debug` — LLM-friendly debug bundle with run state, finish
  reason/detail, trace tail, progress deltas, the latest persisted planner
  decision, honest timeline markers, frame URL, and artifact references.
- `GET /v1/runs/{id}/artifacts` — artifact metadata only; large bytes are never
  embedded.
- `GET /v1/runs/{id}/artifacts/{name}/content` — inline artifact bytes only.

The inspector scans the existing durable finish dumps and verifies the embedded
`run_id`, so a run can still be inspected after its history row was deleted
from the in-memory dashboard.

The private pokeui allowlists these routes. It never exposes them through the
public spectator process.

## Artifact browser

The operator page includes a **Run Inspector** section. Selecting a run shows:

- status, attempt, goal, stop reason, location/frame, runner build and LLM stats;
- artifact name, media type, size, storage location type, and SHA-256;
- downloads routed through PokePilot rather than direct bucket credentials;
- the compact debug JSON used by agent tooling.

Remote artifact content is streamed by `pokereplay`, which first resolves the
object key from pokewall. There is no generic arbitrary-key S3 endpoint.

## `.gbrun` replay player

`run.gbrun` is canonical. MP4 is a disposable derived cache.

1. The UI requests `POST /v1/runs/{id}/replay/render`.
2. pokereplay resolves `run.gbrun` from pokewall.
3. It downloads the recording, verifies the artifact SHA-256, and invokes
   `gomeboy-stream` with the read-only Pokémon Red ROM.
4. GomeBoy restores the checked start state, replays the recorded input timeline,
   and rejects a ROM/model/state/final-hash mismatch.
5. FFmpeg encodes the regenerated frames to MP4.
6. The MP4 is uploaded to the same S3 attempt directory under an immutable key:

   `replay-<first-12-recording-sha256>.mp4`

7. `GET /v1/runs/{id}/replay/video` streams the cached MP4 with HTTP Range
   semantics, so the browser's normal `<video>` controls can seek.

Replay status is available from `GET /v1/runs/{id}/replay/status` with states
`missing`, `generating`, `ready`, `error`, or `disabled`.

Deleting a derived MP4 is safe; it can be regenerated from `run.gbrun`.

## MCP tools for debugging agents

When `POKEPILOT_MCP_TOKEN` enables the existing private MCP server, two new
read-only tools are available:

- `pokepilot_get_run_debug(run_id)`
- `pokepilot_get_run_artifacts(run_id)`

They deliberately return structured metadata rather than giant recording bytes.
An autonomous debugging agent can first inspect the compact bundle, identify the
relevant run/build/progress/failure evidence, and only hand off or request deeper
artifact work when needed.

The existing `pokepilot_get_run`, triage, and investigation tools continue to
work unchanged.

## Deployment

`deploy/farm.yml` adds one private `replay` service. It receives:

- `http://wall:8080` as its metadata catalog;
- the same S3 tuple as runners;
- `${POKEMON_RED_ROM}` mounted read-only at `/rom/pokemon_red.gb`.

The farm image now contains `pokereplay`, the GomeBoy `gomeboy-stream` helper,
FFmpeg, and `intel-media-driver` (iHD). When `/dev/dri/renderD128` is present,
`pokereplay` encodes through `h264_vaapi`; otherwise it stays on `libx264`.
`POKEPILOT_REPLAY_ENCODER=off` forces software. The wall still has neither ROM
nor S3 credentials.

If S3 is not configured, the replay service stays healthy and reports replay as
disabled. Dashboard, farm execution, finish dumps, inline artifact browsing,
MCP run-debug reads, and the public spectator remain independent of replay.

## Current telemetry boundary

The inspector only reports facts PokePilot has actually persisted. Today that
includes the latest persisted planner question/decision, finish trace tail, and
early/final progress snapshots. It does **not** fabricate a decision for every
historical frame. A future semantic event stream can add arbitrary frame-level
LLM/state inspection without changing the artifact/replay ownership model above.
