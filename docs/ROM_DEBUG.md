# Real-ROM debugging

PokePilot keeps normal CI ROM-free. Real Pokemon Red execution lives in a
separate `Real-ROM Debug` GitHub Actions workflow so builds and releases never
need to fetch a commercial ROM.

## Runner ROM setup

The workflow runs on the existing `[self-hosted, linux, pokepilot-rom]` runner.
It resolves the ROM in this order:

1. repository variable `POKEMON_RED_ROM_PATH`, when that file exists;
2. `$HOME/.config/pokepilot/pokemon_red.gb` on the runner;
3. download to that private path from `POKEPILOT_ROM_URL`.

`POKEPILOT_ROM_URL` may be a repository secret (preferred for a signed/private
URL) or a repository variable. The URL is never printed. `scripts/ensure-rom.sh`
verifies the exact supported Pokemon Red SHA-1
`ea9bcae617fdf159b045185467ae58b2e4a48b9a` before the ROM can be used.
Consequently the download happens only when the persistent runner does not
already have the verified file.

For local development the same helper can seed the normal gitignored ROM path:

```sh
POKEPILOT_ROM_URL='https://example.invalid/pokemon_red.gb' \
  bash scripts/ensure-rom.sh roms/pokemon_red.gb
```

## Manual debug runs

Open **Actions -> Real-ROM Debug -> Run workflow**. Three modes are available:

- `test` runs a ROM-backed Go package, optionally restricted with an exact or
  regex `go test -run` pattern;
- `scripted` boots a deterministic PokePilot scripted run with starter,
  destination, and seed inputs;
- `llm` runs the real objective loop with goal, seed, and objective-cap inputs.

LLM mode first reads `$HOME/.config/pokepilot/env`, matching the Makefile's
local runner behavior. It can be overridden with repository secret/variables
`POKEPILOT_DEBUG_LLM_URL`, `POKEPILOT_DEBUG_LLM_MODEL`, and secret
`POKEPILOT_DEBUG_LLM_TOKEN`.

## Agent-driven runs

An agent that can push GitHub branches can trigger the same workflow without a
manual Actions click. Create a branch named `debug/<name>` and optionally add
`.github/rom-debug.env` to that branch. The file is parsed as data, not sourced.
Only these keys are accepted:

```text
MODE=test
TEST_PACKAGE=./skill
TEST_PATTERN=^TestGymBoulderBadge$
SEED=0
STARTER=squirtle
DESTINATION=viridian pokemon center
GOAL=elite-four
MAX_ROUNDS=40
```

Omitted values use the workflow defaults. This gives coding agents a practical
loop: create a `debug/...` branch, push the candidate fix plus a focused debug
request, inspect the Actions log/artifact, then iterate.

## Diagnostics and replay material

The uploaded `rom-debug-*` artifact is intentionally safe to inspect remotely.
It contains the command log, request metadata, exit code, hashes of recent save
states, and automatic `TestProbe` output for up to eight newest failure or
checkpoint states.

ROM bytes, `.state`, `.sav`, `.gb`, and `.gbc` files are never uploaded. Exact
save-state/checkpoint material stays under
`$HOME/.local/state/pokepilot/debug/<run-id>-attempt-<n>` (or
`POKEPILOT_DEBUG_RESULTS` when configured) on the self-hosted runner. The
workflow summary records that path for local replay.
