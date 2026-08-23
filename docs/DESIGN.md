# PokePilot — Technical Design

Status: design phase. No PokePilot code exists yet.
Companion repo: GomeBoy at `/home/maestro/Documents/projects/gomeboy` (we control it).

All findings below come from reading GomeBoy's source and from running the real
Pokémon Red ROM headlessly. Measurements were taken on this machine
(AMD Ryzen 9 7950X, Go 1.24.2).

---

## 1. GomeBoy investigation findings

### 1.1 A headless library API already exists

`pkg/gomeboy` (added in commit `08b3bfc`, "added headless gomeboy library") is a
small facade over `internal/gameboy.GameBoy`. It already provides almost exactly
the API this project was going to ask for:

| Capability | Symbol | Notes |
|---|---|---|
| Construction | `New(...Option)`, `WithROM`, `WithBootROM`, `Headless()` | `Headless()` only stops APU sample accumulation; the core has no GUI/audio |
| ROM loading | `LoadROM(path)` | path only |
| Stepping | `StepFrame()`, `StepFrames(n)` | frame granularity only |
| Input | `Press(Button)`, `Release(Button)` | 8 buttons, held until released |
| Memory | `Read8(addr)`, `Read(addr, len)` | routed through the CPU bus read |
| Video | `Frame() Frame` | zero-copy RGB24 view, 160x144, reused buffer |
| Reset | `Reset()` | reuses in-memory ROM, preserves battery RAM |
| Save states | `SaveState() ([]byte, error)`, `LoadState([]byte)`, `QuickSave/QuickLoad` | gob-encoded full machine state |
| Teardown | `Close()` | flushes cartridge RAM to `.sav` |

Emulator internals live under `internal/`: `io.Bus` (flat 64 KiB `data` array plus
WRAM/VRAM bank arrays and the cartridge), `scheduler.Scheduler` (event-driven,
cycle-accurate), `cpu.CPU` (`Frame()` runs until the frame flag is set),
`ppu.PPU` (pixel FIFO), `apu.APU`, `timer.Controller`, `serial.Controller`.

`pkg/display/{fyne,glfw,web}` and `pkg/audio` hold every frontend concern and are
**not** imported by `pkg/gomeboy`. Frontend/audio separation is already clean.

### 1.2 Save states already exist and are complete

`internal/gameboy/state.go` composes `cpu.State`, `scheduler.State`, `io.State`,
`ppu.State`, `apu.State`, `timer.State`, `serial.State` and the model.
`io.State` carries the entire 64 KiB address space, all 7 WRAM banks, both VRAM
banks, full cartridge state (MBC1/MBC7/HuC1/M161/RTC/camera register state, cart
RAM, ROM/RAM bank offsets), joypad button state, IME, boot flag, and the
DMA/HDMA transfer state. This is a genuine emulator snapshot, not a `.sav`.

**This was expected to be the biggest missing piece. It is already done.**

### 1.3 Determinism is real (measured)

- No `time.Now`/`time.Since` anywhere in the emulator core. The MBC3 RTC ticks off
  `scheduler.Cycle()`, not wall clock (`internal/io/cartridge.go:updateRTC`).
- WRAM power-on noise uses a fixed seed (`wRAMSeed`), not a random one.
- No mutable package-level state in bus/cpu/ppu/scheduler; all state hangs off the
  instance. Multiple instances are genuinely independent.

Measured on `roms/pokemon_red.gb`:

```
ROM 1 048 576 bytes, sha256 5ca7ba01...96b7b, sha1 ea9bcae6...48b9a
header: title "POKEMON RED", cart type 0x13 (MBC3+RAM+BATTERY), ROM 1 MiB, RAM 32 KiB, SGB flag set

600 frames in 278ms  => 2153 fps => 36.1x realtime, one instance, one core
two independent instances, identical framebuffer after 600 frames: true
SaveState: 292 224 bytes; save/load/replay-100-frames reproduces the frame exactly: true

BenchmarkStepFrame-32     474 219 ns/op        0 B/op        0 allocs/op
BenchmarkSaveState-32   1 789 089 ns/op  2 075 191 B/op      104 allocs/op
BenchmarkLoadState-32   2 738 522 ns/op  2 073 269 B/op   24 878 allocs/op
BenchmarkRead8-32              ~ns/op            0 B/op        0 allocs/op
```

36x realtime with zero allocations per frame is comfortably enough; 100 instances
on 16 cores is roughly 200x realtime aggregate. No performance work is needed.

### 1.4 Input works and holds correctly (measured)

Driving the real ROM and reading Gen-1's HRAM joypad mirrors (`hJoyHeld` $FFB4,
`hJoyPressed` $FFB3, `hJoyInput` $FFF8) confirms every button registers with the
correct Gen-1 bit encoding and stays held across frames:

```
hold A     -> hJoyInput=01 hJoyHeld=01 hJoyPressed=01   (auto-repeat decays as expected)
hold DOWN  -> hJoyInput=80 hJoyHeld=80 hJoyPressed=80 -> 00 after 3 frames
hold RIGHT -> hJoyInput=10 hJoyHeld=10
hold START -> hJoyInput=08 hJoyHeld=08
release    -> hJoyInput=00
```

Note `Bus.Press` raises the joypad interrupt unconditionally, ignoring the P1
select bits. Real hardware only fires when the selected line goes low. This is a
minor accuracy deviation; it does not affect Pokémon Red.

### 1.5 The test infrastructure is excellent

`tests/` runs blargg, mooneye, samesuite, dmg-acid2/cgb-acid2, age, scribbl and
little-things-gb ROMs, with image comparison. `pkg/gomeboy` has its own tests for
determinism, reset, save-state replay, instance independence and headless
operation, plus benchmarks. Any change proposed below has an obvious place to be
tested.

### 1.6 What is missing or wrong

**(a) No side-effect-free read.** `Bus.Read` is not pure:

- WRAM/VRAM/ROM reads return `b.dmaConflict` or a locked-region value when the PPU
  holds a region lock or an OAM DMA is in flight (`internal/io/bus.go:492`).
- `$FF00-$FFFF` reads dispatch through `lazyReaders` closures. `STAT`, `DIV`,
  `HDMA5`, `NR52`, wave RAM and the CGB `PCM12/PCM34` readers all *compute* from
  live component state.
- Cartridge RAM reads for MBC7 and POCKETCAMERA call `readMBC7RAM` /
  `readCameraRAM`, which mutate mapper state.

For Pokémon Red (MBC3, DMG) the practical exposure is narrow — the realistic
failure is a WRAM read at a frame boundary that collides with an active OAM DMA
and silently returns the conflict byte instead of the real value. That is exactly
the class of bug that costs days to find. `Bus.Get(addr)` already does the right
thing internally; it is simply not exposed.

**(b) No ROM or cartridge metadata access.** `GameBoy.ROM` and `Bus.Cartridge()`
are behind `internal/`. `pkg/gomeboy` exposes neither ROM bytes nor title, hash,
mapper, ROM/RAM size or CGB flag. PokePilot needs all of it.

**(c) Save files are written to the process CWD, keyed by ROM basename.**
`internal/gameboy.Init` calls `emulator.NewSave(g.filename, ...)`, which does
`os.Create(title + ".sav")` in the working directory. Running the probe above
created `pokemon_red.sav` next to it. Every emulator instance running the same ROM
therefore opens and, on `Close`, overwrites *the same file*. `Init` also probes
for `<basename>.cheats` in the CWD. This silently corrupts state as soon as two
instances run concurrently, and it is a direct blocker for the 100-instance goal.

**(d) No `Clone`.** Branching currently costs a gob round trip: 1.8 ms to save,
2.7 ms to load, 24 878 allocations on the load path. That is ~10 frames of
emulation time per branch. An in-memory `Snapshot`/`Restore` clone would be far
cheaper, and the component-level `Snapshot`/`Restore` methods it needs already
exist — only the gob layer is slow.

**(e) No ROM-bytes constructor.** `New(WithROM(path))` funnels through
`pkg/utils.LoadFile`, which pulls `github.com/bodgit/sevenzip`, brotli, `net/http`
and — via the same package — `golang.design/x/clipboard` (cgo/X11) into the
dependency graph of a headless library. It builds fine with `CGO_ENABLED=0`, but
a headless emulator library should not transitively depend on a clipboard.

**(f) No frame or cycle counter exposed.** Useful for logging, timeouts and replay
alignment; `scheduler.Cycle()` exists internally.

**(g) No debug hooks / breakpoints.** Deliberately fine — see §2.

---

## 2. GomeBoy improvement proposal

Prioritised. Every item answers: what problem, why GomeBoy, what API, how
invasive, accuracy impact, how tested.

### Must have

**GB-A. Per-instance save/cheat file isolation.**
- *Problem*: concurrent instances of the same ROM share one `.sav` path and clobber
  each other; tests litter the CWD; `Close()` writes files nobody asked for.
- *Why GomeBoy*: it is the layer that decides where cartridge RAM is persisted.
  PokePilot cannot work around it without chdir hacks.
- *API*: `WithSaveDir(dir string)`, `WithoutSaves()` (pure in-memory cartridge RAM),
  and make the `.cheats` probe opt-in via `WithCheats(path)`. Default for
  `pkg/gomeboy` should be **no disk I/O at all**.
- *Invasiveness*: small; thread a config struct into `gameboy.Init`.
- *Accuracy*: none.
- *Tests*: two instances of the same ROM in one process write disjoint files;
  `WithoutSaves` creates no files (assert on an empty temp dir).

**GB-B. Side-effect-free introspection (`Peek`).**
- *Problem*: §1.6(a). Observation must never perturb or misreport state.
- *Why GomeBoy*: it owns the memory map, region locks and bank state. Duplicating
  that in PokePilot means reimplementing the bus.
- *API*: on `*Emulator`: `Peek8(addr) byte`, `Peek16(addr) uint16` (little-endian),
  `PeekInto(addr uint16, dst []byte)`. Backed by `Bus.Get`, bypassing region locks,
  DMA conflict and lazy readers. Keep `Read8`/`Read` as the CPU-accurate path and
  document the difference explicitly.
- *Invasiveness*: trivial (~20 lines).
- *Accuracy*: none — it is a new read path, the CPU path is untouched.
- *Tests*: `Peek8 == Read8` for WRAM/HRAM when no lock or DMA is active; during a
  forced OAM DMA, `Read8` returns the conflict byte while `Peek8` returns the true
  value.

**GB-C. ROM access and cartridge metadata.**
- *Problem*: PokePilot must parse map headers, warps, trainers, encounter tables
  and base stats out of the ROM, and must verify it is running a ROM it knows.
- *Why GomeBoy vs. PokePilot loading the file itself*: GomeBoy already decompresses
  `.zip/.gz/.7z`, so a second loader would duplicate that logic **and** could read a
  different file than the one actually running (different path, edited file,
  compressed variant). Serving ROM bytes from the emulator guarantees the parser
  and the running machine agree. The cost is an API that exposes cartridge data —
  acceptable if it is read-only.
- *API*:
  ```go
  func (e *Emulator) ROM() []byte            // read-only view; document "do not mutate"
  func (e *Emulator) ROMSHA256() [32]byte
  func (e *Emulator) Cartridge() CartInfo    // value type, copied out of internal/io
  type CartInfo struct {
      Title, ManufacturerCode string
      CartridgeType           string  // human-readable, e.g. "MBC3RAMBATT"
      MapperCode              uint16
      ROMSize, RAMSize        int
      CGBFlag                 uint8
      SGBFlag                 bool
      HeaderChecksum          uint8
      GlobalChecksum          uint16
      Battery, RAM, RTC, Rumble, Accelerometer bool
  }
  ```
  Do **not** export `*io.Cartridge` — that would leak mutable mapper internals.
- *Invasiveness*: small; a value-copy shim in `pkg/gomeboy`.
- *Accuracy*: none.
- *Tests*: header fields against the known Pokémon Red header
  (`title "POKEMON RED"`, type `0x13`, ROM `0x05`, RAM `0x03`); hash stable across
  `Reset` and `LoadState`.

**GB-D. `WithROMBytes([]byte)` and a dependency diet.**
- *Problem*: §1.6(e). Callers that already hold the ROM (tests, fixtures, embedded
  data) should not go through the filesystem or drag in a clipboard dependency.
- *Why GomeBoy*: it is a constructor.
- *API*: `WithROMBytes(b []byte) Option`; move `LoadFile`'s archive handling into
  its own package (e.g. `pkg/romfile`) so `pkg/gomeboy` no longer imports
  `pkg/utils`; move `clipboard.go` out of `pkg/utils` into the frontend packages.
- *Invasiveness*: small, mechanical; touches import lists only.
- *Accuracy*: none.
- *Tests*: `go list -deps ./pkg/gomeboy` contains no `clipboard`/`sevenzip`;
  `WithROMBytes` and `WithROM` produce identical frames after N steps.

### Should have

**GB-E. `Clone()` and a binary state codec.**
- *Problem*: branching costs a 4.5 ms gob round trip and 25k allocations.
- *Why GomeBoy*: it owns the component `Snapshot`/`Restore` methods.
- *API*: `func (e *Emulator) Clone() (*Emulator, error)` (deep copy via in-memory
  `Snapshot`, no serialization) and a hand-rolled binary encoder replacing gob in
  `SaveState`/`LoadState`, with a magic + version header and a ROM hash field so a
  state cannot be loaded against the wrong ROM.
- *Invasiveness*: moderate. `Clone` is easy; the codec is mechanical but touches
  every `State` struct. Version the format from day one.
- *Accuracy*: none, provided round-trip tests hold.
- *Tests*: clone diverges independently under different input; clone and original
  produce identical frames under identical input; state blob rejects a mismatched
  ROM hash; existing save-state replay tests must still pass.

**GB-F. Frame and cycle counters.**
- *API*: `func (e *Emulator) FrameCount() uint64`, `func (e *Emulator) Cycle() uint64`.
- *Why GomeBoy*: the scheduler already tracks cycles; a frame counter is one `++`.
- *Invasiveness*: trivial. *Accuracy*: none.
- *Tests*: monotonic; `StepFrames(n)` advances the counter by exactly `n`; both are
  captured and restored by save states.

### Nice to have

**GB-G. `SetButton(b Button, down bool)`** — one-line convenience; removes
`if down { Press } else { Release }` from every caller.

**GB-H. Joypad interrupt gating.** Make `Press` respect the P1 select bits. Pure
accuracy improvement, no PokePilot need. Validate against the mooneye joypad tests.

**GB-I. `BenchmarkMultipleInstances`** — N instances stepped in parallel, to keep an
eye on per-instance memory (currently ~300 KB of state plus ROM) and scaling.

### Not needed

- **`StepCycles` / `StepInstruction`.** PokePilot works at frame granularity. Adding
  sub-frame stepping widens the API surface for no gameplay benefit. Revisit only if
  a debugger is built.
- **Memory watch hooks / breakpoints (`WatchMemory`).** A callback on every write is
  either a hot-path cost or a complex range structure. At 36x realtime, polling the
  ~200 bytes PokePilot cares about once per frame is free (`Peek` is a slice index).
  Polling also keeps all game-specific knowledge in PokePilot, which is the boundary
  rule. Skip until profiling says otherwise.
- **Injectable clock (`WithClock`).** The RTC already runs off scheduler cycles and
  no core code reads wall time. Pokémon Red does not use the RTC. There is nothing
  to inject.
- **Pokémon-specific events** (`battle started`, `map changed`). PokePilot's job.
- **Performance work.** 36x realtime, 0 allocs/frame. Leave the emulator alone.
- **Encoding PNG/JPEG in the core.** `Frame()` returning a zero-copy RGB24 view is
  the right primitive; PokePilot encodes PNGs for failure artifacts.

---

## 3. PokePilot technical design

### 3.1 The decisive finding

The ROM at `gomeboy/roms/pokemon_red.gb` is **byte-identical** to the `pokered`
decompilation's build output:

```
sha1 ea9bcae617fdf159b045185467ae58b2e4a48b9a  pokered.gbc          (from ~/.cache/pokered/roms.sha1)
sha1 ea9bcae617fdf159b045185467ae58b2e4a48b9a  roms/pokemon_red.gb
```

A full `pokered` checkout with `pokered.sym` (708 KB of label -> address mappings)
already exists at `~/.cache/pokered`. Every RAM label and ROM routine address is
therefore **known exactly**, not guessed:

```
00:c100 wSpriteStateData1   00:cc26 wCurrentMenuItem   00:cfc4 wFontLoaded
00:cfc5 wWalkCounter        00:d057 wIsInBattle        00:d125 wTextBoxID
00:d163 wPartyCount         00:d16b wPartyMons         00:d31d wNumBagItems
00:d347 wPlayerMoney        00:d356 wObtainedBadges    00:d35e wCurMap
00:d361 wYCoord             00:d362 wXCoord            00:d52a wPlayerDirection
00:ffb3 hJoyPressed         00:ffb4 hJoyHeld           00:fff8 hJoyInput
```

**Design consequence**: PokePilot's memory map is *generated* from `pokered.sym`,
not hand-typed. This removes the single largest source of grind and error in a
project like this. Runtime asserts the ROM hash matches, so a wrong ROM fails
loudly instead of decoding garbage.

### 3.2 The second decisive finding

Blind button mashing does not reliably reach a controllable overworld. Driving the
real ROM, a fixed A/START mash reached Red's House 2F (`wCurMap=0x26`, x=3, y=6) in
~640 frames on one run and stalled in a menu on another with slightly different
timing. Input was proven fine (§1.4); the sequence itself is the problem.

Two consequences shape the whole design:

1. **Every skill is a state machine with a RAM predicate, never a fixed input
   script.** Press, step, check RAM, replan or fail. No blind waits, no
   "press A 40 times and hope".
2. **The intro is solved once and then frozen as a save state.** Reaching a
   controllable overworld is the expensive, flaky part. Do it once, snapshot, and
   never replay it in a test again. This is the highest-leverage use of GomeBoy's
   save states.

### 3.3 Architecture

```
cmd/pokepilot/          CLI: run, replay, dump-state, extract-data
emu/                    GomeBoy wrapper: input scripting, step-until, artifacts
red/                    everything Pokémon Red
  red/sym/              GENERATED address constants (from pokered.sym)
  red/state/            RAM -> typed GameState decoders
  red/rom/              ROM parsers: maps, warps, trainers, encounters, species, moves
  red/data/             derived static tables + type chart (generated, embedded)
world/                  map graph, collision, connections, pathfinding
skill/                  deterministic executors: intro, move, interact, menu, battle
progress/               milestones, objectives, valid next actions
agent/                  LLM planner and tool surface
mcp/                    optional MCP server over agent's tools
```

Deliberately **flatter than the structure in the brief**. Specific departures, each
reversible:

- **No `gen1/` package until a second game exists.** A shared-abstraction layer with
  one implementation is speculative. When Blue or Yellow is added, lift the common
  parts out of `red/` then — the compiler will show exactly what to lift.
- **`navigation`, `interaction`, `menu`, `battle`, `inventory`, `progression` are
  files inside `skill/`, not six packages.** They share the same executor primitive
  (drive input until a RAM predicate holds) and will import each other constantly.
  Split when one grows past ~800 lines.
- **`knowledge/` is not a package.** It is query functions over `red/rom` and
  `red/data`. A separate package would just be a forwarding layer.

### 3.4 The observation loop

```
Peek RAM (one ~256-byte block, once per frame boundary)
      -> decode typed GameState (pure function, no emulator access)
      -> skill executor checks its predicate
      -> if the skill needs a decision it cannot make, ask the planner
```

`GameState` decoding is a **pure function of a memory snapshot**. That makes every
decoder testable from a byte slice with no emulator, and makes failure artifacts
fully reproducible.

```go
type GameState struct {
    Frame     uint64
    Player    PlayerState     // map, x, y, facing, walk counter, movement state
    World     WorldState      // map id, dimensions, tileset, connections, sprites
    Party     PartyState      // 6 slots: species, level, HP, moves, PP, status
    Battle    *BattleState    // nil unless wIsInBattle != 0
    Menu      *MenuState      // nil unless a menu is open
    Dialogue  *DialogueState  // nil unless a text box is up
    Inventory InventoryState  // bag items, money
    Progress  ProgressState   // badges, event flags, milestones
}
```

Rules: nothing outside `red/state` may read a raw address. No `emu.Peek8(0xD361)`
anywhere else in the tree — enforced by a `go vet`-style test that greps for hex
literals outside `red/sym`.

### 3.5 ROM parsing — hybrid, runtime-first

| Data | Source | Why |
|---|---|---|
| Map headers, dimensions, tileset, connections | parsed from ROM at runtime | ROM-hack support falls out for free |
| Blocks, tile collision, warps, signs, objects | parsed from ROM at runtime | same |
| Trainer parties, encounter tables | parsed from ROM at runtime | same |
| Base stats, moves, learnsets, evolutions, items, type chart | parsed from ROM at runtime | all present in the ROM |
| Map *names*, milestone definitions, objective graph | generated Go, embedded | not in the ROM in usable form |

Runtime parsing keeps the binary self-contained (no data files to ship, no ROM
data committed) and makes ROM-hack support a matter of the parser tolerating
different pointer tables rather than a rewrite.

**The decomp checkout is a test oracle, not a runtime dependency.** Golden tests
compare parser output against `~/.cache/pokered`'s `.blk` files and `data/*.asm`
where available, and skip cleanly when that checkout is absent. That gives high
confidence in the parsers without ever depending on the decomp at runtime.

### 3.6 Navigation

Three inputs, cleanly separated:

1. **Static collision** — from the ROM: map blocks -> tileset collision table.
2. **Dynamic obstacles** — from RAM: `wSpriteStateData1`/`2` sprite positions,
   re-read every frame.
3. **Current position** — from RAM: `wCurMap`, `wXCoord`, `wYCoord`.

Pathfinding is A* over a tile graph, with warps and map connections as edges, so
`go_to("Viridian Pokémon Center")` is a single cross-map search rather than a
per-map script.

Execution is step-and-verify, the shape validated in the probe:

```
for each step in path:
    press direction
    step frames until (x,y) changes  or  timeout
    release; step until wWalkCounter == 0
    re-read state
    if position != expected: replan (blocked by an NPC, a text box opened, a
                                     wild battle started, a warp fired)
```

Blocked by a moving NPC: wait a few frames, then re-path around. The planner is
only involved if replanning fails repeatedly or a battle starts. Exact hold/settle
frame counts are calibrated empirically in the first slice and pinned by tests.

### 3.7 Menus, dialogue, battle

All three read state, never pixels.

- **Dialogue**: `wTextBoxID`, `wFontLoaded` and the text-printing flags tell us a box
  is up and whether it is waiting for A. Advance by tapping A and verifying the
  state changed; never a fixed count.
- **Menus**: `wCurrentMenuItem`, `wMaxMenuItem`, `wMenuWatchedKeys` give the cursor
  position and bounds. Selecting item *k* is a closed loop: move cursor, verify
  `wCurrentMenuItem`, press A, verify the menu closed or advanced.
- **Battle**: `wIsInBattle` gates decoding of the enemy mon struct, the player's
  active mon, and the battle menu state. Exposed actions are `use_move(i)`,
  `switch_pokemon(i)`, `use_item(id)`, `run`. The executor drives the menus and
  verifies the outcome (HP changed, turn advanced, battle ended).

The framebuffer is used **only** for failure artifacts and human debugging. No OCR,
no vision model, anywhere in the control path.

### 3.8 LLM boundary

| Deterministic code | LLM |
|---|---|
| RAM decoding, ROM parsing | strategy and objective selection |
| Pathfinding and movement execution | training/team decisions |
| Menu and dialogue navigation | move and item choice in battle |
| Battle menu execution and verification | responses to unexpected situations |
| Type/damage calculation | long-term planning |
| Milestone detection | |

The planner sees a compact, rendered view — objective, location, party summary,
available actions — and returns one high-level action. It never sees raw RAM, a
screenshot, or a button. Knowledge is queried through narrow functions
(`knowledge.Matchup`, `knowledge.Learnset`), never dumped wholesale into context.

### 3.9 Progression

```
GameState -> completed milestones (derived from event flags + badges + party)
          -> valid next objectives (from the objective graph)
          -> planner picks one
          -> objective decomposes into skills
```

Milestones are *detected from state*, never assumed from a script. This is what
lets the agent recover after an unexpected event: it re-derives where it is rather
than losing its place in a macro.

### 3.10 Debugging and testing

Failure artifacts, written on any unexpected state:

```
failure/<timestamp>/
  state.json     decoded GameState
  memory.bin     raw peeked RAM block (replays decoding offline)
  actions.json   recent action/skill trace
  emulator.state GomeBoy save state — replay the exact failure
  frame.png      for humans only
```

The `emulator.state` file is the important one: a bug report becomes a runnable
regression test.

Testing:

- Decoders, parsers, pathfinding, type math: pure unit tests, no ROM, no emulator.
- ROM-dependent tests: gated on `POKEMON_RED_ROM`, `t.Skip` when unset. Never commit
  the ROM.
- Integration tests: start from a **save-state fixture**, run one skill, assert on
  RAM. Fixtures (`pallet_town`, `route_1`, `viridian_pc`, `wild_battle`,
  `trainer_battle`, `menu_open`) are *generated* into a gitignored `testdata/`
  directory on first run and cached — they derive from the ROM, so they are not
  committed either.
- Verification per task: `go build ./... && go vet ./... && go test ./...`.

---

## 4. Dependency diagram

```
GomeBoy                                    PokePilot
-------                                    ---------
GB-A save/cheat isolation ────────┐
GB-B Peek ────────────────────────┼──────> PP-02 emu wrapper ──> PP-05 boot skill
GB-C ROM access + metadata ───────┤              │                      │
GB-D WithROMBytes + dep diet ─────┘              │                      v
                                                 │              PP-06 save-state fixtures
PP-01 skeleton (no deps) ────────────────────────┘                      │
PP-03 sym codegen (no deps) ──> PP-04 state decoders ───────────────────┤
                                        │                               │
                                        v                               v
PP-07 ROM map parser ──> PP-08 world model ──> PP-09 pathfinder ──> PP-10 movement executor
                                                                        │  (SLICE 1)
                                                                        v
                                                   PP-11 interaction ──> PP-12 cross-map nav
                                                                        │  (SLICE 2)
                                                                        v
                                        PP-13 menu executor ──> PP-14 battle decode/execute
                                                                        │  (SLICE 3)
                                                                        v
GB-E Clone + binary codec ──────────────────> PP-17 branching eval      │
GB-F counters ──────────────────────────────> PP-16 logging             v
                                                     PP-15 progression ──> PP-18 agent ──> PP-19 MCP
```

Independent of all GomeBoy work: **PP-01** and **PP-03** (and therefore most of
PP-04's tests, which run on byte slices).

Depends on save states: **PP-06** and everything that uses fixtures — that is, the
fast path for PP-10 onward. Save states already exist, so this is unblocked today;
only PP-17 (branching evaluation) wants GB-E.

GomeBoy critical path is short: **GB-A, GB-B, GB-C, GB-D** are all small and can
land in a day. Nothing in PokePilot beyond PP-01/PP-03 should start before GB-C.

---

## 5. Phased implementation plan

Every task leaves its repository compiling. Verification is
`go build ./... && go vet ./... && go test ./...` unless stated otherwise.

### Phase 0 — GomeBoy foundation

**GomeBoy-01 — Isolate save and cheat file I/O**
- Goal: no emulator instance writes to the CWD unless asked; two instances of one
  ROM never share a file.
- Repo: GomeBoy. Files: `internal/gameboy/gameboy.go`, `pkg/emulator/saves.go`,
  `pkg/gomeboy/gomeboy.go`.
- Details: add a config carrying save dir / disabled-saves / cheats path; thread it
  through `Init`. `pkg/gomeboy` defaults to no disk I/O. Keep existing frontend
  behaviour unchanged by passing the current defaults from `main.go`.
- Tests: `WithoutSaves` leaves a temp dir empty; two instances with distinct
  `WithSaveDir` write distinct files; existing tests still pass.
- Deps: none.

**GomeBoy-02 — Add `Peek8`/`Peek16`/`PeekInto`**
- Goal: side-effect-free introspection that bypasses region locks, DMA conflicts and
  lazy readers.
- Repo: GomeBoy. Files: `pkg/gomeboy/gomeboy.go`, `internal/io/bus.go` (expose a
  peek helper if `Get` is not sufficient for banked regions).
- Details: document `Read*` (CPU-accurate) vs `Peek*` (observer) precisely.
- Tests: agreement with `Read8` in the quiet case; divergence during an active OAM
  DMA; `Peek16` little-endian.
- Deps: none.

**GomeBoy-03 — Expose ROM bytes, ROM hash and cartridge metadata**
- Goal: PokePilot can parse and identify the running ROM.
- Repo: GomeBoy. Files: `pkg/gomeboy/gomeboy.go` (new `cartridge.go`).
- Details: `ROM() []byte`, `ROMSHA256() [32]byte`, `Cartridge() CartInfo` as a value
  type. Do not export `*io.Cartridge`.
- Tests: known Pokémon Red header values; hash stable across `Reset`/`LoadState`.
- Deps: none.

**GomeBoy-04 — `WithROMBytes` and dependency diet**
- Goal: construct from bytes; drop clipboard/sevenzip from the headless graph.
- Repo: GomeBoy. Files: `pkg/gomeboy/gomeboy.go`, new `pkg/romfile/`,
  `pkg/utils/clipboard.go` -> frontend package.
- Tests: `WithROMBytes` and `WithROM` agree after N frames; a dependency test
  asserting `go list -deps ./pkg/gomeboy` contains no `clipboard`.
- Verify: `go list -deps ./pkg/gomeboy | grep -c clipboard` returns 0.
- Deps: none.

**GomeBoy-05 — Frame and cycle counters** *(should-have)*
- `FrameCount()`, `Cycle()`; captured and restored by save states.
- Tests: `StepFrames(n)` advances by exactly n; survives a state round trip.
- Deps: none.

### Phase 1 — PokePilot foundation

**PokePilot-01 — Repository skeleton**
- Goal: module, package layout, CI-able test target, `.gitignore` for ROMs/states.
- Files: `go.mod`, `cmd/pokepilot/main.go`, empty packages with doc comments.
- Tests: a build smoke test. Deps: none.

**PokePilot-02 — GomeBoy adapter (`emu`)**
- Goal: one place that touches GomeBoy. Owns `StepUntil(pred, timeout)`,
  `Tap(button, hold, settle)`, RAM block snapshotting, artifact writing.
- Details: `StepUntil` is the core primitive — step frames until a predicate over a
  fresh RAM snapshot holds, or a frame budget expires (returns a typed timeout error
  carrying the last state).
- Tests: against GomeBoy's `firstwhite.gb` (no Pokémon ROM needed) for stepping,
  input and timeout semantics.
- Deps: GomeBoy-01..04.

**PokePilot-03 — Generate `red/sym` from `pokered.sym`**
- Goal: every address constant is generated, never typed.
- Details: `go:generate` tool reading a `.sym` file, emitting typed constants for
  the WRAM/HRAM labels PokePilot uses, plus the ROM bank:addr pairs for data tables.
  Commit the generated file; the `.sym` input is a dev-time dependency.
- Tests: generated constants match a hand-checked sample (`wCurMap == 0xD35E`,
  `wPartyCount == 0xD163`, `hJoyHeld == 0xFFB4`); generator is idempotent.
- Deps: none. **Can start immediately, in parallel with all GomeBoy work.**

**PokePilot-04 — `GameState` decoders**
- Goal: pure `Decode(mem []byte) GameState`.
- Details: player, world, party, inventory, progress first; `Battle`, `Menu`,
  `Dialogue` as nil-able sub-decoders. No emulator import in this package.
- Tests: table-driven over recorded memory blobs; a decoder must never panic on
  arbitrary bytes.
- Deps: PokePilot-03.

**PokePilot-05 — Boot-to-overworld skill**
- Goal: reach a controllable overworld deterministically, verified from RAM.
- Details: a state machine over title screen -> main menu -> Oak intro -> name
  selection (choose a preset, do not use the naming keyboard) -> rival name ->
  overworld. Each transition has a RAM predicate and a frame budget.
  **This is known to be the flakiest part of the project** (see §3.2) — budget time
  for it and write it defensively.
- Tests: ROM-gated; asserts `wCurMap == 0x26`, sane coordinates, `wFontLoaded == 0`,
  and that a subsequent D-pad hold changes `wXCoord`/`wYCoord`.
- Deps: PokePilot-02, PokePilot-04.

**PokePilot-06 — Save-state fixtures**
- Goal: never boot the intro in a test again.
- Details: fixture generator producing `testdata/fixtures/*.state` from the ROM on
  demand, cached and gitignored; helper `fixture.Load(t, "pallet_town")` that skips
  when the ROM is absent.
- Tests: a fixture loads and decodes to the expected `GameState`.
- Deps: PokePilot-05.

### Phase 2 — Vertical slice 1: read, path, move, verify

**PokePilot-07 — ROM map parser** — map headers, dimensions, tileset id, blocks,
collision, warps, signs, objects, connections, for one map first then all.
Golden-tested against `~/.cache/pokered` `.blk` files (skipped if absent).
Deps: GomeBoy-03, PokePilot-03.

**PokePilot-08 — World model** — tile grid with collision, warps and connections as
a graph. Pure, ROM-data-driven. Deps: PokePilot-07.

**PokePilot-09 — Pathfinder** — A* over the tile graph, dynamic obstacles as
temporary blockers. Pure, unit-tested on synthetic maps. Deps: PokePilot-08.

**PokePilot-10 — Movement executor** — step-and-verify walking; calibrates and pins
hold/settle frame counts; replans on mismatch. Fixture-based integration test:
walk a known route in Red's House / Pallet Town and assert final coordinates.
Deps: PokePilot-06, PokePilot-09. **Completes slice 1.**

**PokePilot-11 — Interaction** — face a target tile, press A, detect the resulting
text box, advance dialogue to completion, return the outcome.
Deps: PokePilot-10.

### Phase 3 — Vertical slice 2: cross-map navigation

**PokePilot-12 — Cross-map navigation** — warp and connection traversal;
`go_to(place)` resolving named destinations. Integration test: Pallet Town ->
Route 1 -> Viridian City -> Pokémon Center -> nurse, entirely from RAM.
Deps: PokePilot-11.

### Phase 4 — Vertical slice 3: menus and battle

**PokePilot-13 — Menu executor** — cursor-state-driven selection for the start menu,
bag, party and shop menus. Fixture-tested. Deps: PokePilot-06, PokePilot-11.

**PokePilot-14 — Battle decode and execute** — `BattleState` decoding plus
`use_move / switch / item / run` execution with verification. Fixtures for a wild
and a trainer battle. Deps: PokePilot-13.

### Phase 5 — Progression, agent, integration

**PokePilot-15 — Progression** — milestone detection from event flags/badges;
objective graph; `available_actions(state)`. Deps: PokePilot-14.

**PokePilot-16 — Structured logging and failure artifacts** — the event set from
§3.10; artifact writer. Deps: PokePilot-02 (+ GomeBoy-05 for counters).

**PokePilot-17 — Branching evaluation** *(optional)* — try N actions from one state
via `Clone`, score outcomes. Deps: GomeBoy-06 (Clone), PokePilot-15.

**PokePilot-18 — Agent** — planner prompt, compact state rendering, tool schema,
decision loop, LLM-free replay mode for testing.
Deps: PokePilot-15.

**PokePilot-19 — MCP server** — thin wrapper over the agent's tool surface. Core
functionality must already work as plain Go. Deps: PokePilot-18.

### Deferred GomeBoy work

**GomeBoy-06 — `Clone()` + binary state codec** — before PokePilot-17.
**GomeBoy-07 — `SetButton`, joypad interrupt gating, multi-instance benchmark** —
opportunistic.
