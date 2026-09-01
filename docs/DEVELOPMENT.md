# Development workflow

PokePilot has two very different feedback loops: ROM-free infrastructure/unit work and ROM-backed gameplay validation. Keeping them separate makes changes faster to review and avoids treating an emulator journey as the default unit-test loop.

## Fast ROM-free loops

Use the smallest target that covers the area you changed:

```sh
make test-farm   # farm/, pokewall, pokeui, deploy
make test-agent  # agent/ and cmd/pokepilot
make test-state  # red/state, red/sym, world
```

All three force `POKEMON_RED_ROM` empty and use `-short`, so they are safe in clean worktrees and CI-like environments.

Before opening or merging a PR, run:

```sh
make verify
```

`make verify` is the repository ROM-free gate: `go vet`, the complete short test suite, then the complete short race suite. It is intentionally slower than the focused targets.

## When the ROM is actually required

Use ROM-backed tests only when the change can alter emulator/gameplay behavior: journey execution, battle/menu timing, generated fixtures, runtime map/collision behavior, or other state that must be measured against the game.

Do not make infrastructure changes depend on a ROM merely because `cmd/pokepilot` imports the emulator. The short suite is expected to remain usable with `POKEMON_RED_ROM` unset.

For gameplay debugging and reproduction rules, `AGENTS.md` remains canonical. In particular, use failure save states and probes instead of assuming a second journey run reproduces the first RNG path.

## Choosing the right package loop

- Farm protocol, leases, persistence, checkpoints, worker presence, operator API: `make test-farm`
- Planner decisions, objectives, observation contracts, runner orchestration: `make test-agent`
- RAM decoding, symbol addresses, world graph/pathfinding helpers: `make test-state`
- Cross-cutting or dependency changes: go directly to `make verify`

If a change spans two areas, run both focused targets during development and `make verify` once before review.

## Test conventions

Prefer behavioral invariants over implementation-detail assertions. Farm tests should describe lifecycle guarantees such as “a queued run is leased once after restart” or “a stale attempt cannot settle the current attempt.” Client tests should assert wire behavior at the HTTP boundary rather than private helper internals.

Use fuzz tests for parsers, path/identifier encoding, JSON boundaries, and other inputs where arbitrary bytes or strings should never panic the service. Seed fuzz tests with real edge cases so normal `go test` runs exercise useful examples even when fuzzing is not enabled.

Keep ROM-derived facts out of ROM-free fixtures. If a test needs actual game truth, mark it as ROM-backed and follow the fixture/probe rules in `AGENTS.md`.

## Formatting

The repository currently contains pre-existing files that `gofmt -l` reports. Avoid mixing a repo-wide formatting sweep with behavioral changes. New or edited Go code should still be formatted normally; a dedicated formatting-only cleanup can remove the historical debt without obscuring functional diffs.
