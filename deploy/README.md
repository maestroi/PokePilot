# PokePilot farm

One image, three processes: `pokepilot` (runner), `pokewall` (orchestrator),
`pokeui` (operator console). The ROM is never in the image; runners bind-mount
it at runtime.

## Use the wall

After `make farm-up`, open **http://localhost:18080**.

- The bar shows running / queued / idle-worker counts.
- **Queue a run** sets planner (`scripted` or `llm`), starter, destination,
  seed, fps, and budgets. Scripted walks starter → dest; llm lets the model
  pick objectives. Idle runners lease the next spec.
- Live cards show the Game Boy screen, map/tile, trace, and **Cancel**.
- Finished runs stay in history with the stop reason.

The browser only talks to `pokeui`. The wall and runners stay on the overlay;
`pokeui` proxies `/v1/dashboard`, `/v1/triage`, `/v1/specs`, cancel, and
`/frame`.

## Local single-node Swarm

Needs Docker Swarm on this machine and `roms/pokemon_red.gb` (or
`POKEMON_RED_ROM`). LLM key comes from `.env` (`llm_token`), same as
`make run-llm`.

```sh
make farm-up                 # build, deploy 1 wall + 1 ui + 2 runners
# UI: http://localhost:18080/
make farm-down
```

`--resolve-image never` uses the image `make farm-image` loaded locally.
A multi-node Swarm cannot see that image store: CI on `main` publishes
`ghcr.io/maestroi/pokepilot` (`.github/workflows/publish-farm.yml`).
Rollout is a timer on the manager (`deploy/pull-latest.sh`) that pins
services to the new digest. Keep Traefik hosts, node bind-mounts, and
tokens out of git.

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
