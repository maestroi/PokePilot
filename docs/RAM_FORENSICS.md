# RAM forensics

PokePilot can preserve the exact 64 KiB Game Boy address space when an objective fails after touching the emulator. This is debugging evidence only: it does not change planner choices, retry failures, answer menus, or make an objective succeed.

## Capture failures

Set `POKEPILOT_RAM_DIR` on a run:

```sh
POKEPILOT_RAM_DIR=./artifacts/ram \
  go run ./cmd/pokepilot -planner llm -fps 0
```

Each gameplay failure writes a pair such as:

```text
failure-frame-0000048123-go-to-route-3.ram
failure-frame-0000048123-go-to-route-3.json
```

The `.ram` file is exactly 65,536 bytes: one byte for every address from `0x0000` through `0xFFFF`. The JSON sidecar records the frame, objective, original error, map/tile, controllability, battle state, and decoded menu cursor fields.

Capture happens inside `agent.Execute` on the error return, before `agent.Run` calls its between-round dialogue recovery. That keeps transient menu and map-transition state from being erased before it can be inspected. Objective validation errors are not captured because no gameplay happened.

## Capture stalls

A failure capture only fires when an objective returns an error. The costlier
pathology is the opposite one: every objective succeeds and the run still goes
nowhere. MEASURED 2026-09-02, rounds 75-82 of a live run alternated Pewter
City and Pewter Gym, eight `-> done` results in a row, and the fact that
explained it (the Boulder badge was already held, so `Offer` correctly
withheld the gym challenge) sat in RAM the whole time with nothing to write
it out.

So the same `POKEPILOT_RAM_DIR` also captures on the edge of the planner's
replan signal — the moment no observable progress has been made for
`strategicReplanAfter` rounds:

```text
stall-frame-0000051004-explore.ram
stall-frame-0000051004-explore.json
```

The sidecar's `kind` is `planner_stall`, its `objective` is the carried
intent, and its `error` is the replan reason. One capture per stall episode,
not per stalled round; observable progress re-arms it.

`failure-` and `stall-` files are separate eviction rings, each holding
`POKEPILOT_RAM_KEEP` pairs, so a burst of objective failures cannot evict the
rarer stall evidence.

Forensic failures are best-effort. An inability to write evidence is logged but never replaces the gameplay error that the planner receives.

By default PokePilot keeps the latest 32 RAM/JSON pairs in the directory. Override that bounded ring with `POKEPILOT_RAM_KEEP`:

```sh
POKEPILOT_RAM_DIR=./artifacts/ram POKEPILOT_RAM_KEEP=8 \
  go run ./cmd/pokepilot -planner llm -fps 0
```

## Compare captures

`cmd/ramdiff` compares two captured address spaces without requiring a ROM:

```sh
go run ./cmd/ramdiff before.ram after.ram
```

Limit the comparison to a suspected region and cap noisy output:

```sh
go run ./cmd/ramdiff \
  -start 0xC000 -end 0xDFFF -limit 100 \
  before.ram after.ram
```

The output is intentionally address-first:

```text
0xCC26  00 -> 01
0xCC27  03 -> 04
2 changed address(es) in 0xC000..0xDFFF
```

Once an address is understood, add it to the generated/symbol-backed `red/sym` and decode it in `red/state` rather than teaching the forensic layer Pokemon-specific meaning. Captured `.ram` files can then be copied into test fixtures so decoder work does not need to reproduce the original emulator timing.

## Emulator boundary

GomeBoy owns only the generic primitive: a side-effect-free full-memory snapshot paired with its frame count. PokePilot owns when a gameplay state is considered worth preserving, how evidence is retained, and how Pokemon-specific RAM is decoded.
