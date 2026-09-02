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
