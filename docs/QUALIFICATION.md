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
| milestone | `fuchsia-koga-surf-strength` | private checkpoint | blocked by #33 |
| milestone | `silph-sabrina` | private checkpoint | blocked by #34 |
| milestone | `cinnabar-blaine` | private checkpoint | blocked by #35 |
| milestone | `viridian-giovanni` | private checkpoint | blocked by #36 |
| milestone | `victory-road-indigo` | private checkpoint | blocked by #37 |
| milestone | `elite-four-champion` | private checkpoint | blocked by #38 |
| full | `fresh-hall-of-fame` | fresh emulator boot | runner available; product completion blocked by #39 |

Future milestones stay in the catalog with their blocker instead of being silently skipped and presented as green coverage.

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
  elite-four-champion/
    start.state
```

A landed checkpoint case fails if its `start.state` is absent. It does not downgrade missing replay evidence to a skip.

For direct checkpoint cases, `pokequal` copies the exact input state into the run evidence, records its SHA-256, loads it through `emu.LoadState`, runs the Red-owned progression skill, and verifies a positive semantic postcondition through `agent.Observation`. A nil skill error alone is never qualification success.

Current direct postconditions are:

- Rocket Hideout: semantic bag owns `silph scope`.
- Pokémon Tower: semantic bag owns `poke flute`.

When later progression slices land, add the smallest positive semantic postcondition for that milestone and flip its catalog entry to runnable.

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

The workflow runs milestone qualification daily. A weekly fresh-save job exists but is gated by:

```text
POKEPILOT_FULL_QUALIFICATION=1
```

Do not enable that repository variable as a required scheduled barrier until #39 has landed. Manual `full` runs are still useful while #39 is being completed because they produce the exact failing frontier and checkpoint ring.

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
