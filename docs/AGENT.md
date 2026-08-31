# The agent loop

`pokepilot -planner` chooses how objectives are picked. Two modes:

- `scripted` (default) — the deterministic flow: boot, take the `-starter`
  Pokemon, walk to `-goto`, keep serving. No model is ever called; a plain
  `make run` works with the inference server switched off.
- `llm` — the objective loop (`agent.Run`). Each round the model picks one
  of the offered objectives: `take a starter` plus one `go to <place>` per
  name `skill.Place` accepts. Every round is printed to stdout, so an
  unattended run leaves a log a human can read in the morning.

Every run is bit-identical unless you ask for otherwise. Gen 1 has no seed:
its RNG is `hRandomAdd`/`hRandomSub` ($FFD3, $FFD4) reseeded from DIV, which
counts CPU cycles, and nothing in the loop reads a wall clock — so the same
ROM and the same inputs meet the same Pidgey on the same tile. `-seed N`
burns N-derived idle frames after boot, which shifts the cycle count and
reroutes every encounter that follows. Measured, walking to Viridian:

    -seed 0   first wild battle at (14,7)    frame 18956
    -seed 1   first wild battle at (12,22)   frame 17721
    -seed 2   first wild battle at (10,32)   frame 16020
    -seed 1   first wild battle at (12,22)   frame 17721   (identical, as intended)

A seed is for run diversity, not for a fair comparison: two policies under
different seeds have different luck, so a difference in outcome is not
evidence. Branch both arms from one save state for that.

Environment (llm mode only):

- `POKEPILOT_LLM_URL` — OpenAI-compatible base URL, default
  `http://192.168.50.204:8000/v1`
- `POKEPILOT_LLM_MODEL` — model name, default `qwen3.5-4b`
- `llm_token` — bearer token for that server, read from `.env`, which
  `make run-llm` sources. Empty means no `Authorization` header is sent.

Every model call logs one line: how many objectives were offered, how long
it took, the raw reply, and what that reply resolved to.

    llm: 6 offered, 260ms, reply "1" -> take a starter

The run stops on budget exhaustion (rounds or frames), on a planner reply
that names no offered objective, or on a failed objective. The final line
says which; `error` and `stuck` stops exit non-zero.

Note: in llm mode the starter objective is always Squirtle
(`agent.Execute`); `-starter` only applies to scripted mode.

Worked example:

    POKEPILOT_LLM_URL=http://localhost:8002/v1 make run-llm

Note: in llm mode a "go to" objective uses skill.Travel, so a wild
encounter on the way is fought and the route resumes.

## ROM facts

Facts about the ROM that planning and tests may rely on, verified against
the vendored decompilation in `pokered/`.

- **Pokemon Center PC:** tile (13,3) of every Center, faced UP. It is a
  `hidden_event` (`OpenPokemonCenterPC`), NOT an NPC sprite, so searching
  the map object files for a PC sprite finds nothing and is misleading.
  S6-4's RUNNOTES claimed "this decompilation's pokecenters have no PC
  machine sprite" and used that to abandon its acceptance test; the claim
  is false — the PC is present in every Center (VIRIDIAN, PEWTER,
  CERULEAN, ...) as a tile-activated hidden event, and depositing a mon
  IS possible in-rom.

- **`map_const` dimensions are in BLOCKS; player coordinates are in TILES,
  and a block is 2x2 tiles.** So `map_const MT_MOON_1F, 20, 18` and a probed
  40x36 AGREE, and `map_const ROUTE_2, 10, 36` and a probed 20x72 AGREE.
  Neither is stale. Three separate tasks (S8-4, and S8-9 twice) have read a
  probe against the decomp, seen the factor of two, and reported the decomp
  as wrong for this ROM. It is not: multiply by two before comparing. Do not
  "correct" either number.

## badgerun: the scoreboard

`cmd/badgerun` answers the slice's question — given verbs and an
observation, does the planner get badge 1? — by running the llm planner to
the Boulder Badge, N times per starter, across several seeds, and printing
a table. It is a harness, not a service: no session registry, no pool,
no UI. It is NOT part of `go test ./...` (its tests cover argument parsing
and table formatting only); a real scoreboard needs a ROM and a live model.

    set -a; . ./.env; set +a
    POKEMON_RED_ROM=roms/pokemon_red.gb \
        go run ./cmd/badgerun -starter all -n 3 -seeds 1,2,3

Per run it reports: starter, seed, badge yes/no, frames to badge (emulated
frames, never wall clock), planner calls, objectives attempted and failed,
battles, blackouts, and where the run stopped. The harness takes the
starter itself — that is the controlled variable; from there the model
decides everything, and nobody tells it the answer (no "take Squirtle",
no Caterpie hint).

- `-seed` varies luck, not skill: idle frames burned after boot shift DIV
  and reroute every encounter. Different seeds are different luck.
- Each run keeps `run.log` (every round line, llm line and outcome line),
  `prompts.txt` (every prompt verbatim — the record of what the model was
  actually told), and `checkpoints/` (S6-11's per-objective ring), so a
  failed run is resumable and inspectable rather than replayed from boot.
- `-inject-fact` appends ONE fact to the system prompt. It is a DIAGNOSTIC
  and defaults OFF: the injected fact is the thing being measured, so on by
  default it would turn the benchmark into a walkthrough. A scoreboard run
  with it on prints a note saying its rows are not comparable to baseline.
- Ablation A (swap the model) is just `POKEPILOT_LLM_MODEL=... \
  POKEPILOT_LLM_URL=...` around the same command.

## Farm evidence and issue handoff

A pokefarm LLM lease writes a bounded checkpoint directory and a periodic
flight recorder. Finish carries `runner_version`, `seed_burn` (zero is a
real value), the final save state, and hashed artifacts: objective
state/knowledge pairs plus recent periodic states. The wall stores those
dumps, groups `error`/`lost` failures by SHA-256 fingerprint, and — when
`-issues-api`, `-issues-project`, and `-issues-ui` are all set — files each
settled occurrence with Agent Orchestrator.

A linked issue number in the farm console is not a PokePilot defect. Agent
Orchestrator may classify the occurrence as expected game/RNG behavior or
external infrastructure. Pokéfarm reports what happened; it does not
declare a code bug.
