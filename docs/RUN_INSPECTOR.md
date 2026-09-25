# Run Inspector, artifacts, and deterministic replay

PokePilot keeps run-debugging ownership inside PokePilot. Agent Orchestrator is
an optional consumer through APIs/MCP; it does not own the PokéFarm run catalog,
S3 object keys, or replay cache.

## Ownership

- **pokewall/PostgreSQL** own run lifecycle state, finish metadata, artifact references, failure/outbox state, and queryable planner exchanges.
- **S3/RustFS** owns large artifact bytes such as `run.gbrun` and derived MP4s.
- **pokereplay** reads artifact references from pokewall, reads/writes S3, and
  has read-only access to the ROM needed by GomeBoy for deterministic replay.
- **pokeui** is the private same-origin relay and Run Inspector UI.
- **Agent Orchestrator** may receive a PokePilot run/issue reference, then use
  PokePilot HTTP/MCP reads when it needs evidence. It never queries a PokePilot
  database or S3 bucket directly.

In production, PostgreSQL is the structured source of truth. Local finish JSON
under PokéWall's cache directory is optional compatibility/debug material only;
inspector correctness does not depend on it.

## Operator HTTP API

Pokewall adds read-only inspection routes alongside the existing wall protocol:

- `GET /v1/runs/{id}` — one live/finished run plus compact finish metadata.
- `GET /v1/runs/{id}/debug` — LLM-friendly debug bundle with run state, finish
  reason/detail, trace tail, progress deltas, the latest persisted planner
  decision, honest timeline markers, frame URL, and artifact references.
- `GET /v1/runs/{id}/artifacts` — artifact metadata only; large bytes are never
  embedded.
- `GET /v1/runs/{id}/artifacts/{name}/content` — inline artifact bytes only.

In PostgreSQL control-plane mode the inspector reads `run_attempts.report_json`
and the `artifacts` index directly. Small inline evidence is reconstructed from
PostgreSQL; large evidence remains an S3 reference. Local finish JSON is read
only by the legacy/local mode.

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
2. pokereplay resolves `run.gbrun` and the matching `media-timeline.json`
   generation from pokewall.
3. It downloads the recording, verifies the artifact SHA-256, and invokes
   `gomeboy-stream` with the read-only Pokémon Red ROM.
4. GomeBoy restores the checked start state, replays the recorded input timeline,
   and rejects a ROM/model/state/final-hash mismatch.
5. The broadcast compositor combines the regenerated game frames with
   deterministic objective, location, badge, party, planner, elapsed-time, and
   semantic-event overlays. Older runs without a media timeline still render
   with unavailable telemetry instead of becoming unreplayable.
6. FFmpeg encodes the composed 1280x720 scene to MP4.
7. The MP4 is uploaded beside the recording under a renderer-versioned immutable
   cache key, so layout changes can re-render historical runs without colliding
   with an older presentation.

8. `GET /v1/runs/{id}/replay/video` streams the cached MP4 with HTTP Range
   semantics, so the browser's normal `<video>` controls can seek.

Replay status is available from `GET /v1/runs/{id}/replay/status` with states
`missing`, `generating`, `ready`, `error`, or `disabled`. Broadcast is
the default mode for status, render, and video. Adding `?mode=raw` to those
three endpoints preserves the game-only renderer and its legacy
`replay-<first-12-recording-sha256>.mp4` cache identity for debugging and
backwards compatibility.

Deleting a derived MP4 is safe; it can be regenerated from `run.gbrun`. A
compositor failure only fails the derived replay job; it never mutates the
source run or recording.

## MCP tools for debugging agents

When `POKEPILOT_MCP_TOKEN` enables the existing private MCP server, three new
read-only tools are available:

- `pokepilot_get_run_debug(run_id)`
- `pokepilot_get_run_recovery_audit(run_id)`
- `pokepilot_get_run_artifacts(run_id)`
- `pokepilot_get_run_artifact_content(run_id, name)`

The run-debug and recovery-audit tools deliberately return structured metadata rather than giant recording bytes. `pokepilot_get_run_debug` remains compact and keeps only the newest timeline events for general debugging; `pokepilot_get_run_recovery_audit` bypasses that 40-event MCP compaction, filters the wall's full bounded activity history down to recovery/failure evidence, annotates attempt revisions when available, and attaches related triage groups including resolved history. This lets an agent audit a long successful campaign without re-investigating already-fixed recovery noise.

`pokepilot_get_run_artifact_content` is that deeper step — the
`.state`/`.ram`/knowledge/failure-repro JSON a triage agent needs to
reproduce a failure locally. It returns base64 bytes bounded by the MCP
response cap and, like the browser's identical content route, resolves
through `pokereplay` when configured so a finish artifact pokewall has
already durabilized to S3 still comes back; only artifacts too large for the
cap (`run.gbrun`) need the operator UI/replay service instead. It exists
specifically because `admin.rompilot.app`'s plain
`GET /v1/runs/{id}/artifacts/{name}/content` route sits behind Cloudflare
Access for browser sessions, while `/mcp` reaches `pokewall`/`pokereplay`
server-to-server and never crosses that edge — so a bearer-token-only agent
uses this tool instead of curling the content route directly.

The existing `pokepilot_get_run`, triage, and investigation tools continue to
work unchanged.

## Deployment

`deploy/farm.yml` adds one private `replay` service. It receives:

- `http://wall:8080` as its metadata catalog;
- the same S3 tuple as runners;
- `${POKEMON_RED_ROM}` mounted read-only at `/rom/pokemon_red.gb`.

The farm image contains `pokereplay`, the GomeBoy `gomeboy-stream` helper,
FFmpeg, and `intel-media-driver` (iHD). `gomeboy-stream` must be installed from
the same GomeBoy release as the runner (see `deploy/gomeboy_pin_test.go`): a
`.gbrun` carries a gob-encoded save state, so a mismatched renderer cannot
restore the recording's start state at all. When `/dev/dri/renderD128` is
present, `pokereplay` encodes through `h264_vaapi`; otherwise it stays on
`libx264`. `POKEPILOT_REPLAY_ENCODER` selects the encoder: `off`/`libx264`
forces software, `vaapi`/`on` forces VAAPI, and anything else (including unset
and `auto`) probes for the render node. `POKEPILOT_VAAPI_DEVICE` overrides the
probed path. The wall still has neither ROM nor S3 credentials.

A multi-attempt run renders one segment per attempt. Each finished segment is
cached in S3 next to its recording before the segments are concatenated, so a
sidecar restart (every image roll recreates it) or a failed attempt only loses
the segments still in flight; the next render request, which the inspector
re-sends automatically while it is open, reuses the rest. Segments render in
parallel, `POKEPILOT_REPLAY_WORKERS` at a time across all jobs (default 3), and
the two-hour render timeout applies per segment.

If S3 is not configured, the replay service stays healthy and reports replay as
disabled. Dashboard, farm execution, PostgreSQL-backed finish inspection, inline
artifact browsing, MCP run-debug reads, and the public spectator remain
independent of replay.

### The multi-node farm runs replay as a device-bound sidecar

On the Swarm overlay the replay sidecar is **not** a stack service, because
Swarm cannot give the iGPU to a service: `devices:` is accepted by the compose
schema but dropped from the task (it starts with `HostConfig.Devices=null` and
no `/dev/dri`), `docker service create` has no `--device` flag, and
`privileged: true` is dropped the same way. Bind-mounting `/dev/dri` as a volume
makes the device nodes visible but opening them fails with `EPERM`, because only
`--device` widens the device cgroup.

So `deploy/replay-sidecar.sh` is the single source of truth for a standalone
container on the one worker that exposes a render node, and it joins the
attachable `pokefarm_gpu` overlay as `replay` for `pokeui` and the spectator.
That script ships inside the image; `deploy/replay-pull.sh` extracts and runs it
from the exact pulled digest, and `pokefarm-replay-pull.timer` on that worker
reconciles it every couple of minutes. The manager's `rollout-latest.sh` only
rolls stack services — it has no SSH trust into the iGPU worker, so it must not
try to roll the sidecar itself.

The sidecar reads its credentials from a host-owned environment file (default
`/opt/pokefarm/replay.env`) carrying the S3 tuple, `LIBVA_DRIVER_NAME`, and
`POKEPILOT_REPLAY_ENCODER`. Secrets stay out of the image and out of the stack
file.

## Current telemetry boundary

The inspector only reports facts PokePilot has actually persisted. Today that
includes the latest persisted planner question/decision, finish trace tail, and
early/final progress snapshots. It does **not** fabricate a decision for every
historical frame. A future semantic event stream can add arbitrary frame-level
LLM/state inspection without changing the artifact/replay ownership model above.

## Run catalog

Production PokéWall keeps run history, attempts, experiments, failures/outbox,
issue links, artifact metadata, and planner exchanges in PostgreSQL. RAM holds
the live working set; local SQLite/`state.json` remain development/compatibility
paths only. Large immutable payloads stay in S3-compatible object storage, with
their hashes and object references in PostgreSQL.
