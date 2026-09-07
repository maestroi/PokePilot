---
name: gomeboy-forensics
description: Use when an objective failure or planner stall needs root-causing from a captured .ram/.state/.json forensic bundle — runs the instruction-level TestDebugProbe to find the exact CPU step that changed a watched byte.
---

# GomeBoy forensic debug probe

PokePilot captures forensic evidence to `POKEPILOT_RAM_DIR` on objective
failures and planner stalls (see `docs/RAM_FORENSICS.md`). Each bundle is
three files sharing a basename: `<prefix>-frame-<n>-<objective>.ram`,
`.state`, `.json`. `TestDebugProbe` (`skill/debug_probe_test.go`) replays the
`.state` file and steps the CPU instruction-by-instruction until a watched
byte changes, reporting the exact PC/opcode/CPU/PPU boundary.

This is diagnostic-only: `StepInstruction`/`WatchMemoryChange` (`emu/debug.go`)
never touch gameplay's `StepFrame`/`StepFrames` path, hooks, or pacing.

## Where to find the bundle

`POKEPILOT_RAM_DIR` is not a fixed path — check in this order before
concluding no evidence exists:

1. **This session's own env** — `echo $POKEPILOT_RAM_DIR` if you ran
   PokePilot yourself in this conversation.
2. **A local ad-hoc run** — the operator set `POKEPILOT_RAM_DIR` by hand per
   `docs/RAM_FORENSICS.md`; ask, or check the run's command line/logs.
3. **A farm run** (leased via `cmd/pokepilot -planner llm`) — the worker
   auto-points `POKEPILOT_RAM_DIR` at `<checkpoint-dir>/ram` (see "Farm runs
   upload bundles automatically" in `docs/RAM_FORENSICS.md`). For the
   default ephemeral `-checkpoint-dir` that directory is removed after the
   run finishes, so the local copy is gone once the run reports its
   outcome — the bundle only survives as an occurrence artifact already
   uploaded to Agent Orchestrator (a separate service; not reachable from
   this repo/session). Ask the user for the occurrence's artifact bundle
   rather than searching the filesystem for it.
4. **Nothing found locally and it's a farm run** — say so plainly and ask
   whether the user can pull the bundle from Agent Orchestrator, rather than
   diagnosing from the error string alone and presenting it as RAM-verified
   (grep the error against source instead — that's a legitimate fallback,
   just not the same rigor as replaying the boundary).

## When to reach for this

- An objective failed or the planner stalled, and `docs/RAM_FORENSICS.md`
  evidence exists in the run's `POKEPILOT_RAM_DIR`.
- You know (or can guess) a WRAM byte that should change when the
  objective/stall condition resolves — e.g. `wJoyIgnore`, a menu cursor, a
  flag byte — and need to know which instruction changes it first.

## How to run it

```sh
PROBE_STATE=<dir>/<prefix>-frame-<n>-<objective>.state \
PROBE_WATCH=wJoyIgnore \
PROBE_CPU_STEPS=100000 \
POKEMON_RED_ROM=/path/to/pokemon_red.gb \
go test ./skill -run TestDebugProbe -v
```

- `PROBE_STATE` — path to the `.state` file from the bundle (checked or
  legacy raw states both load via `emu.LoadState`).
- `PROBE_WATCH` — a label from the vendored `red/sym/testdata/pokered.sym`,
  or a raw address (`0x1234`, `$1234`, or decimal).
- `PROBE_CPU_STEPS` — optional, defaults to 100,000.
- All three env vars plus `POKEMON_RED_ROM` are required; the test `t.Skip`s
  otherwise, so it's safe in normal `go test ./...` runs.

## Reading the output

The log lines report, in order: the loaded state's frame/cycle/ROM hash/state
hash, the watched byte's starting value, then the instruction boundary that
changed it — `PC before -> PC after`, opcode, cycle/frame count, and full
CPU/PPU state before and after. Use the `PC before` value to find the
instruction in `pokered.sym`/disassembly and the CPU registers to reconstruct
what the game code was doing.

If it doesn't find a boundary within `PROBE_CPU_STEPS`, the test fails with
the stopped PC and value — raise `PROBE_CPU_STEPS` or confirm the watched
address is the right one before assuming a real hang.
