# PokePilot farm

One PokePilot image provides four service roles: `pokepilot` (runner), `pokewall`
(orchestrator), `pokeui` (private operator console), and `pokeui -spectator`
(public read-only watch surface). The farm also keeps a separate internal
LiteLLM service available for routing experiments, but normal runners call the
inference hosts directly. The ROM is never in an image; runners bind-mount it
at runtime.

## Use the wall

After `make farm-up`, open **http://localhost:18080** for the operator console.
The public spectator surface is **http://localhost:18081** by default.

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

Normal farm routing is deliberately simple and direct. PokePilot keeps the
existing `llm_profile` wire values for queued/history compatibility, but the
operator-facing meanings are now:

| Operator choice | Wire profile | Route |
| --- | --- | --- |
| 7900 XTX (default) | `auto` | direct 7900 XTX → CPU/LAN after a transport failure or 120s request timeout |
| RTX 4090 | `gpu` | direct 4090 only |
| CPU only | `default` | direct LAN CPU only |

The 4090 is never borrowed automatically. The Operations tab can set the
default for new runs on that Operator browser and the New Run form can override
it per run. Already-running or already-leased runs keep the route they started
with; cancel/requeue one if a GPU must be freed immediately.

Default physical backends are:

```text
7900 XTX  qwen3.8-27B  http://192.168.50.130:8002/v1
4090      qwen3.8-27B  http://192.168.50.81:8002/v1
LAN CPU   qwen 4B      http://192.168.50.204:8000/v1  (bearer llm_token)
```

The normal direct endpoint variables are `POKEPILOT_LLM_GPU_*` for the 7900,
`POKEPILOT_LLM_4090_*` for the 4090, and the historical `POKEPILOT_LLM_*`
variables for CPU/LAN. The farm defaults the 7900 request timeout to `120s`;
the existing failover router then pins the run to CPU/LAN if that direct GPU
ask times out (or encounters another transport-level failure).

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
make farm-up                 # build + deploy farm, LiteLLM, operator/spectator UIs, and 2 runners
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

## Issue handoff (optional)

Qualifying farm failures can be filed automatically with Agent Orchestrator.
This is off unless all three values are set in `.env` (or the environment
`make farm-up` inherits). Empty values leave the farm unchanged.

The public Agent Orchestrator host is the same for API and UI — the UI
proxies `/api` to the controller. These are operator-provided values, not
image defaults:

```
AGENT_ORCHESTRATOR_API=https://orchestrator.labstack.cc
AGENT_ORCHESTRATOR_UI=https://orchestrator.labstack.cc
AGENT_ORCHESTRATOR_POKEPILOT_PROJECT_ID=<pokePilot project uuid>
```

This slice adds no authentication secret.

Reachability from a wall task (alpine's busybox `wget`; `curl` is equivalent
from any host that can see that hostname):

```sh
docker exec "$(docker ps --filter name=pokefarm_wall --format '{{.ID}}' | head -1)" \
  wget -qO- https://orchestrator.labstack.cc/api/health
```

Look up the PokePilot project UUID:

```sh
curl -sS https://orchestrator.labstack.cc/api/projects
```

Use the matching project's `id`. A linked issue number in the farm console is
not proof of a PokePilot defect: Agent Orchestrator may classify the
occurrence as expected game/RNG behavior or external infrastructure.

## Local qwagent triage (optional)

A user systemd timer can offer one unused `GET /v1/triage` group to local
`qwagent` (`opencode run --auto --model qwen3.8-27b/qwen3.8-27b`) every 30
minutes. The picker is deterministic; the model only reproduces, patches, and
opens a PR. It never merges and never writes `main`.

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
`127.0.0.1:8002`, and `roms/pokemon_red.gb` in the worktree or
`~/.config/pokepilot/pokemon_red.gb`. Open PRs are titled
`fix(farm): … [triage:<key>]` so a later tick skips that key.
