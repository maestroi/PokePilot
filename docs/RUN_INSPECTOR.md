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

When `POKEPILOT_MCP_TOKEN` enables the existing private MCP server, three new
read-only tools are available:

- `pokepilot_get_run_debug(run_id)`
- `pokepilot_get_run_artifacts(run_id)`
- `pokepilot_get_run_artifact_content(run_id, name)`

The first two deliberately return structured metadata rather than giant
recording bytes: an autonomous debugging agent first inspects the compact
bundle, identifies the relevant run/build/progress/failure evidence, and only
requests deeper artifact work when needed.

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
