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
`pokeui` proxies `/v1/dashboard`, `/v1/specs`, cancel, and `/frame`.

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
A multi-node Swarm cannot see that image store: publish `FARM_IMAGE` to a
registry you control and point the stack at that reference. Keep registry
names, Traefik hosts, and node bind-mounts out of git.
