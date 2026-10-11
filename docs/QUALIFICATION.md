# ROM-backed qualification

Public pull-request CI is intentionally **ROM-free**. A green `Tests / ROM-free verification` job means the generic/runtime/unit surface is healthy; it does **not** claim that a complete Pokémon Red campaign was replayed.

ROM-backed qualification is a separate private validation layer for deterministic gameplay replays and milestone/full-campaign barriers. It follows the architecture rule:

```text
endless discovery
  -> structured failure + checkpoint
  -> smallest deterministic replay
  -> owning abstraction fix
  -> qualification case
  -> resume exploration
```

## Local command

`cmd/pokequal` verifies the supplied ROM before running any case. The canonical verification is `red/rom.Verify`; unsupported ROM bytes fail before an emulator or test starts.

```sh
export POKEMON_RED_ROM="$HOME/.config/pokepilot/pokemon_red.gb"
export POKEPILOT_QUALIFICATION_CORPUS="$HOME/.config/pokepilot/qualification"

go run ./cmd/pokequal -list
go run ./cmd/pokequal -profile skills -out qualification-out
go run ./cmd/pokequal -profile milestones -out qualification-out
go run ./cmd/pokequal -case pokemon-tower -out qualification-out
go run ./cmd/pokequal -profile full -out qualification-out
```

If `POKEMON_RED_ROM` is not set, the local default is `$HOME/.config/pokepilot/pokemon_red.gb`. If the corpus variable is not set, the default is `$HOME/.config/pokepilot/qualification`.

Never commit the ROM, `.sav` files, or `.state` files. The qualification command never copies the ROM into its output.

## Starting qualification from RomPilot

Operator-driven benchmark runs use the normal **New Run** form in RomPilot.
Choose a **Qualification target** and, when desired, set **Benchmark runs** to
queue a repeated seeded set. These are ordinary farm specs and use the same
workers, model deployment selection, run policy, checkpoints, failure handling,
and spectator path as any other UI-started run.

Each completed Pokémon Red LLM run automatically carries a versioned
`benchmark-result.json` finish artifact. Selecting a qualification target adds
a deterministic semantic end condition and groups repeated runs with the
existing experiment identity fields. Seeds advance from the form's base seed,
so the same base seed and run count can be reused for a fair baseline/candidate
comparison.

The `pokebench` command remains available for private CI/offline execution and
`pokebench compare` remains the machine-readable/result-set comparison tool;
it is not required to launch normal operator qualification runs.

## E2E speed and reliability benchmark

`cmd/pokebench` is the structured measurement layer on top of the same
`agent.Run`, semantic progression, checkpoint, and farm failure-fingerprint
systems used by qualification. It does not parse screen text and it does not
change controller timing to obtain measurements.

A fresh baseline and candidate comparison looks like:

```sh
go run ./cmd/pokebench red \
  --mode speedrun \
  --from fresh \
  --until hall-of-fame \
  --runs 3 \
  --seed 1 \
  --output ./benchmarks/baseline

# make the routing/model/runtime change

go run ./cmd/pokebench red \
  --mode speedrun \
  --from fresh \
  --until hall-of-fame \
  --runs 3 \
  --seed 1 \
  --output ./benchmarks/candidate

go run ./cmd/pokebench compare \
  ./benchmarks/baseline \
  ./benchmarks/candidate
```

The same seed sequence is used when the same `--seed` and `--runs` are
supplied, so baseline and candidate see the same deterministic fresh-run frame
burns. Use `--seeds 7,11,13` when an exact seed list is preferred.

For a focused regression, load a preserved semantic milestone checkpoint
instead of replaying Pallet Town onward:

```sh
go run ./cmd/pokebench red \
  --from checkpoint:/absolute/path/to/sabrina.state \
  --until blaine \
  --runs 3 \
  --output ./benchmarks/sabrina-to-blaine
```

Named checkpoints resolve against `POKEPILOT_QUALIFICATION_CORPUS` in both
the existing `<case>/start.state` layout and the benchmark
`checkpoints/<milestone>.state` layout. Benchmark-created milestone
checkpoints copy the paired agent knowledge/coverage files when they exist, so
a replay resumes with the same semantic agent memory rather than only emulator
RAM.

### Result contract

Each run writes a version-1 `benchmark-result.json`. The containing directory
includes game, fresh/checkpoint benchmark type, end milestone, commit, timestamp,
run index, and seed. The result records:

- commit, verified ROM SHA-256, mode, seed, source/checkpoint hash, and sanitized
  model/run policy identity;
- canonical emulator frames and emulated seconds separately from wall time;
- semantic major-milestone splits with absolute/delta frames, wall splits,
  party, map, badges/capabilities, and the objective crossing the split;
- coarse frame attribution (navigation, battle, menus, healing,
  shopping/inventory, field actions, unclassified) plus strategist/fast
  inference wall latency;
- planner/model call counts, p50/p95 latency, token totals, route and health;
- optimization counters derived from existing structured objective evidence;
- farm-compatible structured failure fingerprints, recent objective/events,
  planner/navigation diagnostics, semantic state, and a replay checkpoint.

No endpoint credential/token is persisted. Endpoint URLs are stripped of
userinfo, query strings and fragments, and arbitrary settings pass through a
secret-key denylist before serialization.

A failed run is still written before the command returns non-zero. Its console
summary includes the last split, failed objective, farm fingerprint, checkpoint,
and frame count. `benchmark-summary.json` aggregates repeated runs without
folding failures into successful completion time. `pokebench compare` makes
completion-rate changes visible before speed deltas and reports only descriptive
sample comparisons; it does not claim statistical significance from small N.

Pokémon Red owns the milestone definitions in `red/benchmark`; the generic
benchmark engine therefore does not need Red map/event constants. Yellow and
future games can provide another profile using the same result/aggregation
format.

## Qualification layers

The current catalog lives in `qualification/catalog.go`.

| layer | case | replay input | status |
| --- | --- | --- | --- |
| focused | `rom-short` | verified ROM | runnable |
| milestone | `opening-brock` | generated/cached `forest_north_gate` fixture | runnable |
| milestone | `mt-moon-cerulean` | generated/cached `mt_moon_b2f` fixture | runnable |
| milestone | `misty` | generated/cached `post_boulder` fixture | runnable |
| milestone | `rocket-hideout` | private `rocket-hideout/start.state` | runnable |
| milestone | `pokemon-tower` | private `pokemon-tower/start.state` | runnable |
| milestone | `fuchsia-koga-surf-strength` | private checkpoint | runnable |
| milestone | `silph-sabrina` | private checkpoint | runnable |
| milestone | `cinnabar-blaine` | private checkpoint | runnable |
| milestone | `viridian-giovanni` | private checkpoint | runnable |
| milestone | `victory-road-indigo` | private checkpoint | runnable |
| milestone | `elite-four-loss-recovery` | private losing Indigo checkpoint | runnable; required defeat -> blackout -> restart -> League recommit |
| milestone | `elite-four-champion` | private checkpoint | runnable; save/reopen/load after every League stage |
| full | `fresh-hall-of-fame` | fresh emulator boot | runnable; #39 closes only after a clean proof |

Closed story slices stay in the daily milestone profile. A missing private checkpoint is a qualification failure, not a green skip.

`mt-moon-cerulean` is the regression barrier for the farm defect that stalled
run `run-17rjs2d1uf1kw3` for eighty rounds. Mt. Moon B2F's fossil corridor is
two tiles wide, its Super Nerd stands on one of them, and he steps aside only
after a fossil is taken — so the crossing is progression, not geometry. The
case starts from the `mt_moon_b2f` fixture and asserts the whole transaction:
the exit refuses with a named missing `can_exit_mt_moon` capability, the owned
objective satisfies it, a second run of the objective buys no second fossil,
and Cerulean is then ordinary travel. `misty` crosses the same gate as part of
its longer road.

The Brock and Misty journey tests use the existing versioned fixture system. `pokequal` points `POKEPILOT_FIXTURE_DIR` inside the run output, so the exact start fixture used by that qualification run remains beside its evidence. Rocket Hideout and Pokémon Tower start from preserved private corpus states because their focused story journeys do not have committed save-state fixtures.

## Private checkpoint corpus

Recommended layout:

```text
$POKEPILOT_QUALIFICATION_CORPUS/
  rocket-hideout/
    start.state
  pokemon-tower/
    start.state
  fuchsia-koga-surf-strength/
    start.state
  silph-sabrina/
    start.state
  cinnabar-blaine/
    start.state
  viridian-giovanni/
    start.state
  victory-road-indigo/
    start.state
  elite-four-loss-recovery/
    start.state
  elite-four-champion/
    start.state
```

A landed checkpoint case fails if its `start.state` is absent. It does not downgrade missing replay evidence to a skip. `pokequal` preflights private checkpoints before starting either a direct skill case or a Go-test case, copies the exact input into that case's private evidence directory, and records its SHA-256. Explicit `-corpus` overrides are also pinned into the child-test environment, so local and self-hosted runs resolve the same corpus path deterministically.

For direct checkpoint cases, `pokequal` then loads the preserved state through `emu.LoadState`, runs the Red-owned progression skill, and verifies a positive semantic postcondition through `agent.Observation`. A nil skill error alone is never qualification success. Go-test checkpoint cases retain the same private `start.state` evidence and hash while the focused test owns the stronger multi-step assertions.

The `elite-four-loss-recovery` case starts from an intentionally underpowered
Indigo-lobby checkpoint that deterministically loses to Lorelei with the normal
battle policy. It requires a typed `RequiredBattleError` with encounter identity,
verifies the real Indigo blackout without falsely committing Lorelei, restarts
the emulator from that post-loss state, and proves the ordinary League traversal
heals and recommits to Lorelei. Generic ROM-free agent tests cover the other half
of the contract: that this typed loss normalizes to `combat_defeat` and only
becomes retry-ready after material combat-readiness progress.

The `elite-four-champion` Go-test case is intentionally stronger than a single
in-process gauntlet. After League start, each Elite Four member, the Champion,
and Hall of Fame completion, it serializes the emulator state, closes the
emulator, opens a fresh instance, reloads the checkpoint, and re-asserts the
stage's semantic fact before continuing. This makes checkpoint/resume a
qualification property rather than an assumption.

Current direct postconditions are:

- Rocket Hideout: semantic bag owns `silph scope`.
- Pokémon Tower: semantic bag owns `poke flute`.
- Fuchsia/Koga: semantic progress `fuchsia_progression_complete` (Soul + HM03 + HM04).
- Silph/Sabrina: Marsh Badge is owned.
- Cinnabar/Blaine: Volcano Badge is owned.
- Viridian/Giovanni: Earth Badge is owned.
- Victory Road/Indigo: semantic progress `indigo_plateau_ready` is complete.

## Failure evidence

Each run writes `qualification.json`; each case has its own directory. Depending on the layer, evidence includes:

- exact runner revision/ref and run id;
- verified ROM SHA-1 (never ROM bytes or ROM path in the manifest);
- OS/architecture/Go version;
- LLM profile, endpoint, model and non-secret request settings;
- command/log output;
- exact starting checkpoint and SHA-256 for private checkpoint scenarios;
- final checked save state;
- semantic initial/final observations;
- journey failure state when an existing ROM journey test emits one;
- per-objective checkpoint ring for the full campaign;
- prompt/reply logs and typed `GoalStatus`, LLM route/health, token totals, and final observation for the full campaign.

The full fresh-save case runs `agent.Run` directly with the structured `elite-four` goal. It only passes when the typed run stop is `done` **and** a fresh semantic goal evaluation says the Elite Four/Champion goal is complete. Console text is not parsed as a success signal.

## GitHub Actions

`.github/workflows/qualification.yml` is manual/scheduled only. It has no `pull_request` trigger and targets a self-hosted runner labeled:

```text
self-hosted, linux, pokepilot-rom
```

The runner can use the default private paths above, or repository variables can point to local runner paths:

- `POKEMON_RED_ROM_PATH`
- `POKEPILOT_QUALIFICATION_CORPUS`
- `POKEPILOT_QUALIFICATION_RESULTS`

LLM configuration uses the existing `POKEPILOT_LLM_*`, `POKEPILOT_LLM_GPU_*`, and profile environment variables available to the self-hosted runner. Credentials are never written to qualification metadata.

The workflow runs milestone qualification plus two representative checkpoint benchmarks daily and runs the structured fresh-save Hall-of-Fame benchmark weekly. The full run is intentionally a real barrier: it closes #39 only after the typed eight-badge + Hall-of-Fame postcondition passes. Failures preserve the checkpoint ring and diagnostics so the next frontier can be replayed without weakening the milestone suite.

### Artifact boundary

Replay `.state` files stay in the persistent qualification result directory on the self-hosted runner. The GitHub Actions artifact is intentionally a **safe diagnostic projection** that excludes:

```text
*.state
*.sav
*.gb
*.gbc
```

This keeps exact replay material private/local while still uploading logs, semantic observations, manifests, prompt/reply traces, and hashes for review. The Actions summary prints the runner-local replay directory for operators with access to that host.

## Adding a regression case

When the farm finds a real defect:

1. take the nearest replayable checkpoint associated with the structured failure fingerprint;
2. reproduce the smallest failing skill/objective/milestone;
3. fix the owning abstraction rather than special-casing the historical run;
4. preserve the checkpoint in the private corpus if it is the stable milestone input;
5. add/enable the corresponding catalog case and positive postcondition;
6. run the case directly, then the milestone profile;
7. return to endless exploration only after the regression barrier is green.

### Trainer interruption during destination wait

`TestTrainerArrivalReplay` preserves the regression from
`run-2txl3quu7z1p8juj7uehiayj5`, round 119. Download the private artifact
`round-119-frame-0000814784-go-to-cerulean-gym--fleeing-wild-battles.state`
from that run's inspector into the private corpus, for example
`trainer-arrival/start.state`. The test verifies its SHA-256 before replaying
both TravelFlee and the complete objective transaction:

```bash
POKEMON_RED_ROM="$PWD/roms/pokemon_red.gb" \
POKEPILOT_TRAINER_ARRIVAL_STATE="$POKEPILOT_QUALIFICATION_CORPUS/trainer-arrival/start.state" \
go test ./agent -run '^TestTrainerArrivalReplay$' -v
```

Both gym trainers must be handled and the objective must complete in a
controllable overworld beside the live sprite occupying the destination.
The short suite independently checks interruptions during NPC waits and
rejects adjacent arrival without live occupancy, across maps, during battle,
or while uncontrollable. No save-state or ROM bytes are committed.

## Gen2 combat-loss preparation on the way to the coastal frontier

Gold/Silver's deterministic story is not just a chain of gym flags. Real Gold
farm attempt 54 (`run-2co88t5c8hl0gokvsp7n8nvf`, issue #2550) failed Morty
and the portable combat gate correctly refused to repeat the *same losing
party*. Previously the Gen2 adapter could execute only starter and progress
objectives, leaving no usable recovery action.

Gold/Silver now also execute bounded **heal** and lead **train** actions.
A typed combat failure on the currently unfinished story objective opens a
training rung sized to the recorded party-readiness deficit, using the already
owned Slowpoke Well B1F or Route 37 encounter route. Each rung heals before and
after training; eggs are excluded from HP postconditions. The normal combat
gate remains closed until the observed party actually reaches its readiness
target. This is generic recovery capability for the known Gen2 frontier—not a
guarantee that Morty, Chuck, or Jasmine is winnable with every party.

The first passing post-cure checkpoint and six-badge ROM replay remain the
qualification gates described below. Do not treat ROM-free regression success
or the availability of recovery objectives as evidence of six badges.

## Gold/Silver coastal checkpoint qualification

The Red `pokequal` catalog remains Red-specific. Gold/Silver's fifth/sixth badge
integration has a separate opt-in, **ROM-backed** regression:
`TestGSCoastalROMCheckpointReplay` in `agent` and the manual self-hosted
workflow `.github/workflows/gen2-coastal-qualification.yml`.

The replay starts at a real **post-SecretPotion/Jasmine-cure** checkpoint and
executes the same production objective adapter transactions the game runner uses:

1. Claim HM04 Strength at Olivine Café (durable event required).
2. Travel across Route 40/41 to Chuck, observe a real native **Strength boulder
   move** in Cianwood Gym, win Storm Badge (badge bit 5).
3. Claim HM02 Fly from Chuck's moving wife (durable event required).
4. Return from Cianwood to Olivine and win Jasmine's Mineral Badge (badge bit 4).

Each step starts with an incomplete goal and requires a settled, controllable
overworld, a semantic postcondition, and the correct destination map. The
emulator is closed/reopened and its **checked save state is reloaded after
every milestone**, then the progress fact must still hold. The final observation
must contain Storm, Mineral, and `gs_supported_frontier`. The gym test does
not count Chuck's scripted boulder throw as a Strength push: only movement of
one of his three puzzle boulders is sufficient.

### Private fixture

**Automatic capture is now the preferred source.** When a normal Gold/Silver
runner successfully executes the Jasmine medicine objective, the game-owned
qualification hook writes the first verified, settled `SaveStateChecked` into
the private corpus **immediately after that objective**. It is never replaced
by later runs. `source.json` records the run ID, verified progress objective,
frame, SHA-1 of the ROM and SHA-256 of the checked state (never ROM bytes).

Set `POKEPILOT_QUALIFICATION_CORPUS` on the runner to a private writable
directory that the ROM-qualification runner can also read. The Swarm runner
service now mounts the host directory from
`${POKEPILOT_QUALIFICATION_CORPUS:-/var/lib/pokepilot/qualification}` at
`/var/lib/pokepilot/qualification`; the self-hosted workflow defaults to the
same host path. For multi-node Swarm **this must be shared storage or both
processes must be pinned to the same host**. A container-local volume on one
node cannot be read by a different qualification runner.

The automatic capture is opt-in for standalone agents (unset env means no
writes) and on for the configured Swarm runner. Only the exact first successful
Jasmine cure milestone is eligible: completion event true, HM04 still missing,
no battle or pending dialogue, and the cartridge on Lighthouse 6F. A snapshot
merely resumed with those flags does not count as trusted new source evidence.
On write/mount failure, the run logs a qualification error but does not undo
successful story progress.

For Gold, the automatically captured **real playable checkpoint** is produced
after Jasmine has returned to Olivine Gym but **before HM04 Strength** is
collected:

```text
$POKEPILOT_QUALIFICATION_CORPUS/
  gs-coastal/
    gold/
      post-cure/start.state
    silver/
      post-cure/start.state
```

These are ordinary GomeBoy `.state` or `SaveStateChecked` snapshots. Do not
create them by modifying RAM to grant story flags: the point is to prove the
actual game route from a trusted prior completion boundary. The write is
atomic and first-writer-wins, preventing a later run or incomplete write
from replacing the replay baseline. Ensure the team is
battle-ready and a Surf carrier is available; the replay owns navigation and
field-move preparation but does **not** train an underleveled team for Chuck.

The workflow runs a **daily automatic full Gold qualification attempt**
(`43 5 * * *` UTC), in addition to manual Gold/Silver/both runs. The
scheduled job executes on the private `pokepilot-rom` runner and requires a
genuine, automatically captured `post-cure/start.state` **and**
`source.json`. It verifies metadata schema, game, cure objective, ROM SHA-1,
and state SHA-256 against the actual ROM and save-state bytes before running
any emulator objective. Missing ROM, missing capture or mismatched metadata is
an explicit **failed/blocked workflow**, never a passing or skipped gameplay
qualification. The pending state produces an actionable Actions job summary.

The self-hosted runner needs retail Gold/Silver ROMs matching the pinned
profile SHA-1 and paths configured with `POKEMON_GOLD_ROM_PATH` and
`POKEMON_SILVER_ROM_PATH` repository variables (or files under
`~/.config/pokepilot/pokemon_gold.gbc` and `pokemon_silver.gbc`).
The corpus/result variables are shared with the Red qualification runner.
Scheduled runs use Gold only; invoke the manual workflow with `silver` or
`both` once those checkpoints are available. Manual replay still accepts
older legitimate private checkpoints without `source.json`; if the manifest
does exist it is always validated, even for manual runs.

Run through GitHub Actions → **Gen2 Coastal ROM Qualification** → **Run workflow**
and choose `gold`, `silver` or `both` with `post-cure`. After the farm produces `start.state`, this
manual workflow consumes it directly; no copying from the rotating
`checkpoints/` ring is necessary. To prepare the very first corpus input,
start a normal Gold/Silver run targeting the experimental Gen 2 frontier and
let it complete the medicine objective. The workflow deliberately fails if no
qualifying state has yet been produced. Or run locally:

```sh
POKEPILOT_GS_COASTAL_QUAL=1 \
POKEPILOT_GS_COASTAL_GAME=gold \
POKEPILOT_GS_COASTAL_ROM=/private/pokemon_gold.gbc \
POKEPILOT_GS_COASTAL_START=/private/qualification/gs-coastal/gold/post-cure/start.state \
POKEPILOT_GS_COASTAL_OUT=/private/results/gs-coastal-gold \
go test -count=1 -timeout=90m ./agent -run '^TestGSCoastalROMCheckpointReplay$' -v
```

A failure is not a skip: once qualification is opted into, absent/invalid ROM
bytes, missing/corrupt checkpoint, incorrect initial story state, unmoved gym
boulders, incomplete flags, wrong map, unresolved battle and failed checkpoint
reload all fail the job. No ROM or save-state bytes are committed or uploaded.

### Fast failure reproduction

The runner keeps `01-strength.state`, `02-storm.state` and
`03-fly.state` (plus `failure.state` if an objective fails) on its local
results disk. Copy an earlier successful state into a private corpus subfolder
and select its matching `from` value to avoid replaying completed segments:

| `from` | start state's required milestone | first objective replayed |
| --- | --- | --- |
| `post-cure` | Jasmine returned to Gym | HM04 Strength |
| `post-strength` | HM04 Strength acquired | Chuck / Storm |
| `post-storm` | Storm Badge earned | HM02 Fly |
| `post-fly` | HM02 Fly acquired | Jasmine / Mineral |

Use `gs-coastal/<game>/<from>/start.state` and select that `from` in the
workflow (or set `POKEPILOT_GS_COASTAL_FROM` locally). Shortened replays prove
only the **remaining** stages; a full `post-cure` pass is still required for
the end-to-end coastal frontier. The workflow's uploaded safe artifact includes
only logs and semantic JSON; actual checked states remain on the ROM runner.

**Qualification status:** a merged harness, a scheduled job, and a captured
checkpoint are **not** proof that a retail Gold/Silver coastal replay passed.
Only a real passing `Gen2 Coastal ROM Qualification` job with checked
checkpoint provenance counts. Monitor Actions for `qualification BLOCKED` or
the first real puzzle/travel failure; preserve its runner-local checked state
for focused repair. Deploy/farm storage and ROM runner access must use the
same shared corpus host path.
