# PokePilot

PokePilot plays Pokémon Red headlessly, and measures how far a planner can
get with a menu of game verbs.

It boots the real ROM in [GomeBoy](https://github.com/maestroi/gomeboy), a
deterministic Game Boy emulator, drives it with typed Go executors — boot,
take a starter, travel, battle, gym, shop, heal — and, in `llm` mode, lets a
model pick the next objective from a printed menu. Gameplay truth comes from
RAM, never from pixels; every round is logged; every run is reproducible to
the bit.

## Why it is built this way

- **Determinism is the point.** Gen 1's RNG is reseeded from the CPU cycle
  count (`rDIV`), and nothing in the loop reads a wall clock — so the same
  ROM and the same inputs meet the same Pidgey on the same tile. `-seed N`
  burns N-derived idle frames after boot to get *different* luck, not a fair
  comparison.
- **No pixels.** The browser view is for humans to watch. PokePilot reads
  the game through `red/state` (a RAM snapshot decoded into typed state), so
  assertions are exact and tests cannot flake on a frame.
- **The decomp is vendored.** `pokered/` is the full pokered decompilation,
  byte-identical to `roms/pokemon_red.gb` (sha1
  `ea9bcae617fdf159b045185467ae58b2e4a48b9a`). Every ROM fact this project
  relies on is read from it; `docs/POKERED.md` maps question → file.

## Layout

| Path | What it is |
|---|---|
| `emu/` | The only package that talks to GomeBoy: open, step, input, save states, watch |
| `red/rom/` | Static game data parsed out of the ROM image: maps, warps, connections, collision |
| `red/state/` | A RAM snapshot decoded into typed game state: party, inventory, badges, player, menus, text |
| `red/sym/` | Generated RAM/HRAM addresses, verified against a committed `pokered.sym` snapshot |
| `world/` | The map graph, collision grids, and BFS pathfinding |
| `skill/` | Deterministic executors (boot, starter, goto, travel, battle, gym, shop, …) plus the fixture cache and the probe test |
| `agent/` | The intent layer above `skill`: objectives, observation, and the LLM planner loop |
| `farm/` | Lease/spec types shared by farm runners and the wall |
| `cmd/pokepilot` | The main binary: boots the ROM, serves the screen over HTTP, runs a scripted or llm planner |
| `cmd/badgerun` | Scoreboard harness: llm planner to the Boulder Badge, N times per starter and seed, prints a table |
| `cmd/pokewall` | Farm orchestrator: leases, checkpoints, flight recorder, issue handoff |
| `cmd/pokeui` | Operator console; the browser talks only to this |
| `pokered/` | The vendored decompilation — see `pokered/UPSTREAM.md` |
| `deploy/` | Docker image and Swarm stack for the local farm (`deploy/README.md`) |
| `docs/` | Design, agent-loop notes, decomp map, slice plans |
| `roms/` | Gitignored; your ROM lives here |

## Getting started

Requires Go 1.26 and a Pokémon Red ROM for gameplay. GomeBoy is pinned in
`go.mod` to the PokePilot-maintained fork at `github.com/maestroi/gomeboy`, so
a normal fresh clone has no dependency on a developer-local filesystem path.
ROM-free verification also works without a ROM.

```sh
export POKEMON_RED_ROM=roms/pokemon_red.gb   # roms/ is gitignored
make run                                      # scripted: boot, take the starter, walk to -goto
make run ARGS='-goto "pallet town"'
```

The screen is served at `http://localhost:8099` for humans to watch.

### LLM planner

```sh
make run-llm        # sources .env for llm_token
make run-llm-local  # same loop against a local model, thinking disabled
make run-llm-auto   # prefer local GPU; pin the configured fallback on transport failure
```

Environment: `POKEPILOT_LLM_URL` (default `http://192.168.50.204:8000/v1`),
`POKEPILOT_LLM_MODEL` (default `qwen3.5-4b`), `llm_token` from `.env`. The
`run-llm-auto` target additionally understands `POKEPILOT_LLM_FALLBACK_URL`,
`POKEPILOT_LLM_FALLBACK_MODEL`, `POKEPILOT_LLM_FALLBACK_TOKEN`,
`POKEPILOT_LLM_FALLBACK_TIMEOUT`, and the `AUTO_LLM_*` Make overrides. A
primary transport failure pins the fallback for the rest of the run; ordinary
model rejection/retry does not switch backends.

`-goal` accepts ordinary prompt text, but four structured forms are evaluated
against decoded game state before each model call and stop deterministically
when complete: `badges:N`, `reach:<place>`, `level:N`, `item:<name>`, plus the
`elite-four` goal. For example:

```sh
make run-llm ARGS='-goal badges:1'
make run-llm-auto ARGS='-goal "reach:pewter city"'
```

The default is `elite-four`. Nothing reaches the Champion yet, so a default
run ends on the round cap rather than on success, and each round carries its
own progress ("badges N/8") into the planner's prompt. Use `-goal badges:1`
for a short run that can actually finish.

Each round the model picks one of the offered objectives — `take a starter`
plus one `go to <place>` per name `skill.Place` accepts — and the round is
printed to stdout, so an unattended run leaves a log a human can read in the
morning. The run stops on structured-goal completion, budget exhaustion, a
reply naming no objective, or a failed objective. Details in `docs/AGENT.md`.

### Scoreboard

```sh
POKEMON_RED_ROM=roms/pokemon_red.gb \
    go run ./cmd/badgerun -starter all -n 3 -seeds 1,2,3
```

Per run it reports badge yes/no, frames to badge (emulated frames, never
wall clock), planner calls, battles, blackouts, and where the run stopped;
each run keeps `run.log`, `prompts.txt`, and resumable `checkpoints/`. It is
a harness, not a service — not part of `go test ./...`.

## Testing

```sh
make verify         # ROM-free: module graph, vet, short tests, race tests
make test           # full go test ./...; ROM-backed tests skip without ROM
```

- `world` and `red/rom` tests are pure ROM-byte tests: milliseconds, no
  emulator, cannot flake.
- `skill` journey tests boot the emulator from cached fixtures — save states
  generated on demand from your ROM under `skill/testdata/fixtures/`
  (gitignored; set `POKEPILOT_FIXTURE_DIR` to share a cache). Emulator tests
  skip when `POKEMON_RED_ROM` is unset.
- A failing journey test dumps its final save state to
  `skill/failure/<TestName>.state`. Re-running a journey test is **not**
  reproducing it: the RNG is seeded from the cycle count, so the second run
  is a different game. Read the dump instead:

  ```sh
  PROBE_STATE=failure/TestGymBoulderBadge.state \
      go test ./skill -run '^TestProbe$' -v
  ```

## The probe

`skill.TestProbe` answers geometry questions without reading a collision
grid into context:

```sh
POKEMON_RED_ROM=roms/pokemon_red.gb PROBE_MAP=0x0c PROBE_AT=15,13 \
    go test ./skill -run '^TestProbe$' -v
```

`PROBE_TO`, `PROBE_BLOCK`, `PROBE_ROUTE`, and `PROBE_STATE` extend it.
`AGENTS.md` is the canonical reference for working on this repo.

## The farm

`make farm-up` builds the image and deploys a local single-node Docker
Swarm: one wall (orchestrator), one UI (operator console at
`http://localhost:18080`), two runners. `make farm-down` tears it down. CI
on `main` publishes `ghcr.io/maestroi/pokepilot` for multi-node Swarms.
Details in `deploy/README.md`.

## Documentation

| Doc | What it answers |
|---|---|
| `AGENTS.md` | Working rules: probes, the decomp, RNG, fixtures, what has already cost time |
| `docs/DESIGN.md` | The technical design and the GomeBoy investigation |
| `docs/AGENT.md` | The agent loop, ROM facts, badgerun, farm evidence |
| `docs/POKERED.md` | Question → file map for the vendored decomp |
| `docs/ROAD-TO-ELITE-FOUR.md` | Everything between Cerulean City and the Pokémon League |
| `RUNNOTES.md` | Per-slice run notes |
| `docs/plans/` | Active slice plans |
| `docs/archive/` | Implemented slice plans and designs |

## House rules

- Never commit a ROM or any `.gb` / `.sav` / `.state` file.
- Coordinates come from `skill.Place`, never literals.
- Use `Travel`, not `GoTo`, for anything crossing tall grass.
- Event flag indices come from `state.Event`, never from counting `const`
  lines.

The full list, with the reasoning, is in `AGENTS.md`.
