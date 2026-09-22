# PokePilot farm

One PokePilot image provides five service roles: `pokepilot` (runner), `pokewall`
(orchestrator), `pokeissues` (GitHub issue sink), `pokeui` (private operator
console), and `pokeui -spectator` (public read-only watch surface). The farm also
keeps a separate internal LiteLLM service available for routing experiments, but
normal runners call the inference hosts directly. The ROM is never in an image;
runners bind-mount it at runtime.

## Use the wall

After `make farm-up`, open **http://localhost:18080** for the operator console.
The public spectator surface is **http://localhost:18081** by default.

Production hostnames are documented in `docs/DOMAINS.md`:

```text
rompilot.app          Public RomPilot / spectator
admin.rompilot.app    Private admin/control plane
api.rompilot.app      External API entry point
```

- The operator bar shows running / queued / idle-worker counts.
- **Queue a run** sets planner (`scripted` or `llm`), starter, destination,
  seed, fps, and budgets. Scripted walks starter → dest; llm lets the model
  pick objectives. Idle runners lease the next spec.
- Live operator cards show the Game Boy screen, map/tile, trace, and **Cancel**.
- Finished runs stay in history with the stop reason.
- The spectator page can only watch runs. It exposes no queue, cancel, delete,
  triage, raw dashboard, or MCP routes.

The browser only talks to a `pokeui` process. The wall and runners stay on the
overlay. The private operator process proxies `/v1/dashboard`, `/v1/triage`,
`/v1/specs`, cancel/delete, and `/frame`. The spectator process has its own
server-side route table and sanitized `/v1/watch` contract; see
`docs/SPECTATOR.md`.

## LLM routing

When `POKEPILOT_MODEL_REGISTRY` is set (the farm stack mounts
`deploy/models.json` at `/etc/pokepilot/models.json`), the operator Tools form
selects a first-class **deployment** instead of a legacy LLM profile. Each
deployment copies immutable inference identity onto the run. Switchable hosts
are owned by `pokemodelhost`; the wall waits until that host reports `ready`
before handing a runner lease, and it will not switch models while a lease is
active. The 7900 XTX farm default uses `discover: true`: it probes the live
`/v1/models` endpoint, so `xtx-9b` / `xtx-27b` host switches show up in the UI
and leased run identity without editing the registry.

Paired experiments also carry an explicit comparability identity. `make farm-up`
derives the mounted Pokémon Red ROM SHA-256 and a deterministic hash of the
runtime/prompt-generating Go sources, and passes both to PokéWall. If
`pokemon_blue.gb` is present in `POKEPILOT_ROM_DIR`, its SHA-256 is derived
separately. Override any of these with
`POKEPILOT_ROM_SHA256_POKEMON_RED`,
`POKEPILOT_ROM_SHA256_POKEMON_BLUE`, or `POKEPILOT_PROMPT_SHA256` in
`.env` / `~/.config/pokepilot/env`. Missing or mismatched identities do not
silently enter benchmark totals: the experiment UI marks those seed pairs
non-comparable and excludes them from aggregates.

The existing `llm_profile` wire values remain the compatibility adapter for
queued/history runs and for installations with no registry:

| Operator choice | Wire profile | Route |
| --- | --- | --- |
| Deployment `qwen38-27b-7900` / 7900 XTX (default) | `auto` | direct 7900 XTX → CPU/LAN after a transport failure or 120s request timeout |
| Deployment `qwen35-4b-4090` / `qwen35-9b-4090` / RTX 4090 | `gpu` | direct 4090 only (`api_model` `pokepilot-4090`) |
| CPU only | `default` | direct LAN CPU only |

The 4090 is never borrowed automatically. `pokemodelhost` on that machine is
the sole owner of port 8002 and launches every selectable 4090 model with the
stable alias `pokepilot-4090`. Already-running or already-leased runs keep the
route they started with; cancel/requeue one if a GPU must be freed immediately.

Default physical backends are:

```text
7900 XTX  switchable (discover)  http://192.168.50.130:8002/v1
4090      switchable   http://192.168.50.81:8002/v1  (alias pokepilot-4090; control http://192.168.50.81:8091)
LAN CPU   qwen 4B      http://192.168.50.204:8000/v1  (bearer llm_token)
```

Set `POKEPILOT_MODELHOST_TOKEN` in `.env` or `~/.config/pokepilot/env`. The
token is never stored in the registry, run spec, or git. The 4090 control port
is LAN-only.

The normal direct endpoint variables are `POKEPILOT_LLM_GPU_*` for the 7900,
`POKEPILOT_LLM_4090_*` for the 4090, and the historical `POKEPILOT_LLM_*`
variables for CPU/LAN. The farm defaults the 7900 request timeout to `120s`;
the existing failover router then pins the run to CPU/LAN if that direct GPU
ask times out (or encounters another transport-level failure). The 4090
compatibility route uses `POKEPILOT_LLM_4090_MODEL=pokepilot-4090`.

LiteLLM is still deployed and its backend definitions remain in
`deploy/litellm.yaml`, but `POKEPILOT_LLM_GATEWAY_URL` is empty by default so it
is not in the hot path. Set it explicitly to `http://litellm:4000/v1` to
re-enable the gateway for an experiment. The known LAN model id in the
repository is `qwen3.5-4b`; override `POKEPILOT_LITELLM_LAN_MODEL` if the
server's `/v1/models` endpoint exposes a different qwen-4b id. LiteLLM model ids
use the `hosted_vllm/` prefix so it forwards `chat_template_kwargs`
(`enable_thinking: false`); the `openai/` prefix uses the OpenAI SDK and drops
that field, which leaves Qwen 3.8 on its default `xhigh` thinking path.

## Local single-node Swarm

Needs Docker Swarm on this machine and `roms/pokemon_red.gb` (or
`POKEMON_RED_ROM`). LLM key comes from `.env` (`llm_token`), same as
`make run-llm`.

```sh
make farm-up                 # build + deploy farm, issue adapter, LiteLLM, operator/spectator UIs, and 2 runners
# Operator UI: http://localhost:18080/
# Spectator:   http://localhost:18081/
make farm-down
```

Override the published ports with `FARM_WALL_PORT` and `FARM_SPECTATOR_PORT`.
If a hostname is public, route it only to the spectator port; keep the operator
port private because its HTTP UI can mutate runs even when MCP is disabled.

`--resolve-image never` uses the PokePilot image `make farm-image` loaded
locally. LiteLLM uses its separately pinned upstream image. A multi-node Swarm
cannot see a manager's local PokePilot image store: CI on `main` publishes
`ghcr.io/maestroi/pokepilot` (`.github/workflows/publish-farm.yml`). Rollout is a
timer on the manager (`deploy/pull-latest.sh`) that pins services to the new
digest. Keep Traefik hosts, node bind-mounts, and tokens out of git.

## GitHub issue handoff

Qualifying farm failures are now filed directly in GitHub Issues through the
in-stack `pokeissues` adapter. PokéWall still owns the durable outbox,
fingerprinting, quarantine counters, regression detection, and status sync; the
adapter only translates that protocol into GitHub operations.

Set a fine-grained GitHub token in `.env` or `~/.config/pokepilot/env`, restrict
it to `maestroi/PokePilot`, and grant **Issues: read/write** plus **Contents:
read/write**. Contents write is used only for the long-lived `farm-repros`
prerelease and its portable repro ZIP assets; no Actions or Workflows permission
is required.

```sh
POKEPILOT_GITHUB_TOKEN=<token with Issues + Contents read/write on maestroi/PokePilot>
# Optional; these are the stack defaults:
POKEPILOT_GITHUB_REPO=maestroi/PokePilot
POKEPILOT_RUN_BASE_URL=https://admin.rompilot.app
```

Only the `issues` service receives `POKEPILOT_GITHUB_TOKEN`. The wall, runners,
replay service, operator UI, and spectator never receive it.

GitHub gets a safe issue summary, triage key/fingerprint, selected scalar
evidence, run/debug link, and artifact name/size/hash metadata. When a failure
has a replayable objective checkpoint, `pokeissues` additionally publishes one
content-addressed, ROM-free ZIP under the `farm-repros` prerelease. That bundle
contains only the exact `round-*.state`, its paired `knowledge-vN.json`, bounded
repro metadata, and the matching `failure-repro.json` contract when present.
It deliberately excludes the ROM, credentials, recordings, screenshots, raw
model exchanges, and unrelated run artifacts. Because this repository is
public, the portable repro ZIP is public too; deeper/full binary evidence still
remains in the PokePilot run store.

The issue links the ZIP and includes an offline command such as
`go run ./cmd/pokerepro -bundle <github-release-asset-url> -play`. That path
verifies the embedded state/knowledge hashes and does not need access to
`admin.rompilot.app`.

Issue lifecycle maps cleanly back into the wall:

- open GitHub issue → active failure group; equivalent farm occurrences stay
  quarantined locally instead of creating duplicates;
- closed as **completed** → resolved/fixed; the adapter freezes the default-
  branch revision at the issue's close time as `fixed_revision`. A later
  occurrence from a strict ancestor of that revision is retained as a stale
  recurrence without reopening the issue. An occurrence on the fixed revision
  itself, a newer/diverged revision, or one whose ancestry cannot be proven
  reopens the same issue and adds one idempotent regression comment;
- closed as **not planned** → ignored/not-planned; later equivalent occurrences
  stay quarantined rather than reopening it.

The `Investigate` action in the operator console now adds one investigation
request comment to the GitHub issue. The local qwagent loop still claims work
from MCP `pokepilot_get_triage`, so it does not depend on GitHub issue state
to run.

## Local qwagent triage (optional)

A user systemd timer can offer one unused MCP `pokepilot_get_triage` group to
local `qwagent` (`opencode run --auto --model qwen3.8-27b/qwen3.8-27b`) every
30 minutes. The picker talks to `/mcp` because `admin.rompilot.app/v1/*` sits
behind Cloudflare Access; a bearer token alone gets a 302 login page there.
The picker is deterministic; the model only reproduces, patches, and opens a
PR. It never merges and never writes `main`. Every real coding attempt is
recorded against the failure key with its backend, requested model, outcome,
branch, and PR. The operator failure view shows the latest solver plus the
attempt count; GitHub resolution and PokePilot verification still decide
whether a repair actually succeeded.

OpenCode defaults to `qwen3.8-27b/qwen3.8-27b`. Escalation is deliberately
manual. For a one-off stronger attempt, invoke the script directly so the
model override is scoped to that process, for example
`POKEPILOT_TRIAGE_AGENT=opencode POKEPILOT_OPENCODE_MODEL=<model> ./deploy/qwagent-triage.sh`.
The systemd timer never changes models on its own; `qwtriage-once` continues to
use the defaults from `~/.config/pokepilot/env`.

```sh
make qwagent-triage-install   # units + zsh helpers; timer stays off
qwtriage-on                   # enable the 30-minute timer
qwtriage-off                  # disable and stop
qwtriage-once                 # one attempt, timer unchanged
qwtriage-status
qwtriage-logs
./deploy/qwagent-triage.sh --dry-run   # print the next key; do not claim
```

Needs `POKEPILOT_MCP_TOKEN` in `~/.config/pokepilot/env`, `gh` auth, Qwen on
`127.0.0.1:8002` or an authenticated Cursor CLI, and
`~/.config/pokepilot/pokemon_red.gb` (symlinked into the worktree each tick).
Open PRs are titled `fix(farm): … [triage:<key>]` so a later tick skips that
key.
