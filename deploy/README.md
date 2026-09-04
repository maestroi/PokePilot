# PokePilot farm

One image, four service roles: `pokepilot` (runner), `pokewall` (orchestrator),
`pokeui` (private operator console), and `pokeui -spectator` (public read-only
watch surface). The ROM is never in the image; runners bind-mount it at runtime.

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

## Local single-node Swarm

Needs Docker Swarm on this machine and `roms/pokemon_red.gb` (or
`POKEMON_RED_ROM`). LLM key comes from `.env` (`llm_token`), same as
`make run-llm`.

```sh
make farm-up                 # build + deploy wall, operator UI, spectator UI, and 2 runners
# Operator UI: http://localhost:18080/
# Spectator:   http://localhost:18081/
make farm-down
```

Override the published ports with `FARM_WALL_PORT` and `FARM_SPECTATOR_PORT`.
If a hostname is public, route it only to the spectator port; keep the operator
port private because its HTTP UI can mutate runs even when MCP is disabled.

`--resolve-image never` uses the image `make farm-image` loaded locally.
A multi-node Swarm cannot see that image store, so CI publishes
`ghcr.io/maestroi/pokepilot` (`.github/workflows/publish-farm.yml`). Every pull
request runs the complete Docker build. Pull requests whose head branch is in
this repository also publish the built merge result as both its exact commit
tag and `:latest`; pushes to `main` do the same. The manager timer
(`deploy/pull-latest.sh`) watches `:latest` and pins `wall`, `ui`, `spectator`,
and `runner` to each new digest, so one PR image rolls the complete farm end to
end. Fork pull requests are build-only because their GitHub token is read-only.
Closing a pull request rebuilds the current base branch onto `:latest`, so an
unmerged review cannot remain deployed indefinitely. Keep Traefik hosts, node
bind-mounts, and tokens out of git.

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
