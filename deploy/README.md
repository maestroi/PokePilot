# PokePilot farm

One PokePilot image provides four service roles: `pokepilot` (runner), `pokewall`
(orchestrator), `pokeui` (private operator console), and `pokeui -spectator`
(public read-only watch surface). A separate internal LiteLLM service routes LLM
requests to the available inference machines. The ROM is never in an image;
runners bind-mount it at runtime.

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

The farm runs LiteLLM as an internal-only gateway. PokePilot still stores the
existing `llm_profile` values in run specs, but those values now express resource
intent rather than physical IP addresses:

| Operator choice | Wire profile | Route |
| --- | --- | --- |
| Auto | `auto` | 7900 XTX → 4090 → LAN |
| Reserve 4090 | `gpu` | 7900 XTX only |
| Reserve all GPUs | `default` | LAN only |

The Operations tab can set the default for new runs on that Operator browser.
The New Run form can override it per run. Already-running or already-leased runs
keep the route they started with; cancel/requeue one if a GPU must be freed
immediately.

Default physical backends are:

```text
7900 XTX  qwen3.8-27B  http://192.168.50.130:8002/v1
4090      qwen3.8-27B  http://192.168.50.81:8002/v1
LAN CPU   qwen 4B      http://192.168.50.204:8000/v1  (bearer llm_token)
```

The known LAN model id in the repository is `qwen3.5-4b`; override
`POKEPILOT_LITELLM_LAN_MODEL` if the server's `/v1/models` endpoint exposes a
different qwen-4b id. The LAN llama.cpp server listens on `:8000` and
requires the same bearer as `.env`'s `llm_token` (`POKEPILOT_LITELLM_LAN_KEY`).
Backend URLs/models are configurable with
`POKEPILOT_LITELLM_7900_*`, `POKEPILOT_LITELLM_4090_*`, and
`POKEPILOT_LITELLM_LAN_*`. Model ids must use the `hosted_vllm/` prefix so
LiteLLM forwards `chat_template_kwargs` (`enable_thinking: false`). The
`openai/` prefix uses the OpenAI SDK and drops that field, which leaves
Qwen 3.8 on its default `xhigh` thinking path.

Runners normally call `http://litellm:4000/v1`. The direct LAN endpoint remains
configured as a transport fallback for Auto/LAN profiles if the gateway service
itself is unavailable. Explicit dedicated-GPU mode has no LAN fallback by
design. Set `POKEPILOT_LLM_GATEWAY_URL` empty to return to the historical direct
endpoint routing path.

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

These URLs are one operator's LAN, not image or stack defaults:

```
AGENT_ORCHESTRATOR_API=http://192.168.50.81:8080
AGENT_ORCHESTRATOR_UI=http://192.168.50.81:8081
AGENT_ORCHESTRATOR_POKEPILOT_PROJECT_ID=<pokePilot project uuid>
```

Both services stay LAN-only. This slice adds no authentication secret.

Reachability from a wall task (alpine's busybox `wget`; `curl` is equivalent
from any host that can see that LAN):

```sh
docker exec "$(docker ps --filter name=pokefarm_wall --format '{{.ID}}' | head -1)" \
  wget -qO- http://192.168.50.81:8080/api/health
```

Look up the PokePilot project UUID:

```sh
curl -sS http://192.168.50.81:8080/api/projects
```

Use the matching project's `id`. A linked issue number in the farm console is
not proof of a PokePilot defect: Agent Orchestrator may classify the
occurrence as expected game/RNG behavior or external infrastructure.
