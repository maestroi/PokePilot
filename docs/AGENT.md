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

## Test fixtures

`skill/fixture` caches expensive emulator boot sequences as save states so
journey tests start from a deterministic, millisecond-fast state instead of
re-walking the road on every run. Fixtures are derived from the ROM named by
`POKEMON_RED_ROM`, generated on demand, cached as `<name>.v5.state` under
`ResolveDir()` (gitignored; `POKEPILOT_FIXTURE_DIR` overrides the location,
which is how a clean worktree shares one cache). A cached state is validated
on write AND on read, and the positive contract of every fixture is asserted
by name in `skill/fixture/fixture_test.go` — map, position and progress,
never just "controllable".

The registered set, in road order:

| fixture | stands at | positive contract |
|---|---|---|
| `reds_bedroom` | boot overworld (0x26 (3,6)) | controllable overworld |
| `post_starter` | Oak's lab | `EVENT_BATTLED_RIVAL_IN_OAKS_LAB` set |
| `pallet_town` | `Place("pallet town")` | map + coords |
| `route1` | `Place("route 1")` | map + coords |
| `viridian_city` | `Place("viridian city")` | map + coords |
| `viridian_pokecenter` | `Place("viridian pokemon center")` | map + coords |
| `viridian_mart` | `Place("viridian mart")` | map + coords, parcel delivered |
| `post_errand` | (19,8) on 0x01, north of the gate | parcel delivered, gate crossed |
| `post_pokeballs` | Oak's lab | 5x POKE_BALL, Route 22 rival beaten |
| `forest_north_gate` | (5,1) on 0x2f, the forest's north gate | map + coords, lead level >= 12 (beats Brock) |
| `pewter_city` | `Place("pewter city")` | map + coords |
| `post_boulder` | `Place("pewter city")` | Boulder Badge bit set in RAM, party healed |

The three past-Viridian fixtures (S10-8) are built on each other from a
fresh boot: `forest_north_gate` = post_errand + the road through the forest
+ the forest grind to level 12 (the same session/heal/blackout grind the
gym journey used to run at test time, now in the builder); `pewter_city` =
that + the last road leg; `post_boulder` = that + the Brock fight (the build
FAILS on a loss rather than caching it). Cold build costs, measured 2026-09-01
in fresh cache dirs: forest_north_gate 1m05s, pewter_city 1m06s,
post_boulder 1m15s.

`forest_north_gate` has a second purpose: the gate's standing Super Nerd
(home tile (3,2), `pokered/data/maps/objects/ViridianForestNorthGate.asm`,
sprite 12) is two steps from the fixture's standing tile, so its line ("You
need to look everywhere...") is one Talk away. That is the state the
requirement-harvest test (S10-9) needs — the same line is unreachable from
`post_errand` because the forest leg is blocked by the (2,18) Youngster
stalemate. Verify with the probe:

    PROBE_STATE=.../forest_north_gate.v5.state go test ./skill -run '^TestProbe$' -v

Rules that cost this project time, restated for the next builder:

- Bump `fixtureVersion` when the boot sequence or the definition of a valid
  state changes; a stale `.v4.` file must never load as a `.v5.` one.
- A fixture that depends on a flaky journey is worse than no fixture: the
  builders use the frame-shift retry (123 frames) for the no-encounter
  phase, treat blackouts as recoverable, and `post_boulder` refuses to
  cache a lost fight.
- Never commit a `.state` or `.gb`.

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
battles, blackouts, and where the run stopped — plus a second line per row
with what the run cost and the conditions it ran under: reply retries
(re-asks after a rejected reply), the planner's Health counters (transport
errors, rejected replies, fallback parses), the prompt/completion token
totals (summed over EVERY model call, rejected re-asks included — a re-ask
costs a full prompt), and an 8-hex-char prompt hash. The hash is computed
from the exact values the request is built from (base system prompt, goal,
extra system text, reply schema) — never a hand-maintained version
constant — and is a comparability marker, not a version scheme: no
registry, no hash-to-slice mapping. Two rows with different hashes are not
comparable, and the run's prompts.txt holds the prompt verbatim, which is
the record of what a hash meant. Each run's prompts.txt begins with a
`prompt <hash>` line naming its generation. The harness takes the
starter itself — that is the controlled variable; from there the model
decides everything, and nobody tells it the answer (no "take Squirtle",
no Caterpie hint).

- `-seed` varies luck, not skill: idle frames burned after boot shift DIV
  and reroute every encounter. Different seeds are different luck.
- Each run keeps `run.log` (every round line, llm line and outcome line),
  `prompts.txt` (every prompt verbatim — the record of what the model was
  actually told), and `checkpoints/` (S6-11's per-objective ring), so a
  failed run is resumable and inspectable rather than replayed from boot.
- `-resume <run-dir>` picks a crashed run back up instead of restarting
  amnesiac: it takes the NEWEST checkpoint in `<run-dir>/checkpoints`
  (the lexicographic max — the writer names them
  `round-NNN-frame-NNNNNNNNNN-slug.state`, zero-padded, so name order IS
  round order), restores both the save state and the knowledge captured
  beside it (no re-boot, no starter, no seed burn — the state is the state),
  and writes back into the SAME ring, where eviction still evicts state+
  knowledge pairs together. The run announces the resume in the banner and
  in `run.log`, and its scoreboard row carries `resumed=N`. **Round numbers
  of a resumed run restart at 1**: `resumed=N` means this row's round 1 is
  the original run's round N+1, so a row that began at round 90 never
  pretends to have begun at round 1. The frame numbers in the checkpoint
  names are the absolute coordinate; the round number is a run-relative
  label. Absent flag = today's behaviour, unchanged.
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
