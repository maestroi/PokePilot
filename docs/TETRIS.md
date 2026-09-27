# Tetris support

PokePilot supports **Tetris (World) (Rev 1)** as its first non-Pokémon Game Boy title.

Supported fingerprint:

- SHA-1: `74591cc9501af93873f9a5d3eb12da12c0723bbc`
- SHA-256: `0d6535aef23969c7e5af2b077acaddb4a445b3d0df7bf34c8acef07b51b015c3`

## Runtime boundary

Tetris uses `game.CartridgeProfile` for ROM identity and exposes semantic gameplay state through `tetris.StateProfile`. It does not implement Pokémon's `game.GameProfile`.

`tetris.State` exposes:

- the 10×18 locked-cell board;
- active and preview tetromino identity/orientation;
- the active piece anchor column/row;
- mode, level, Type A score and line progress;
- pause, lock/clear transitions, game-over and completion state;
- `ReadyForPieceInput`, which is false during pause, locking and line-clear transitions.

Revision-owned raw RAM addresses live in `tetris/sym`.

## Deterministic placement execution

`tetris/control.Place` is the low-level closed-loop placement primitive.

A target is:

```go
control.Placement{
    Rotation: 0, // 0..3, matching the game's raw orientation index
    Column:   4, // active-piece anchor column
}
```

The executor:

1. requires a currently controllable falling piece;
2. rotates at the current position using one-frame button edges;
3. verifies every requested orientation change from semantic state;
4. shifts one anchor column at a time and verifies every horizontal change;
5. holds Down for the game's soft drop;
6. detects the lock/clear transition;
7. releases Down and waits for either the next controllable piece or a terminal state;
8. verifies that the placement produced a board, line-progress or terminal-state change.

The original game's controls are used directly:

- **A** decrements the raw orientation index (clockwise in the disassembly);
- **B** increments the raw orientation index (counter-clockwise);
- **Left/Right** move one column per fresh press;
- **Down** performs the soft drop.

The controller chooses the shorter rotation direction. Equal two-turn rotations use A deterministically.

A blocked rotation or horizontal shift returns `control.ErrBlocked` immediately; the controller does not continue with a different placement. Starting during pause/lock/clear state returns `control.ErrNotReady`. A piece that never begins or finishes its lock transition returns `control.ErrTimeout`.

## Placement policy

`tetris/policy.Choose` enumerates placements that the Phase-3 controller can reach from the current semantic piece state, simulates the vertical landing and line clears, evaluates the resulting board, and returns a deterministic `control.Placement`.

The simulator uses exact Rev-1 tetromino geometry transcribed from the game's 4×4 sprite matrices. The semantic piece anchor corresponds to matrix row/column 2, so policy geometry and controller coordinates use the same origin.

Policy objectives are:

- `auto`: Type A chooses score play, Type B chooses line completion, and versus chooses survival;
- `survival`: strongly penalizes holes, stack height, roughness and wells;
- `lines`: increases the immediate value of clearing rows while retaining safety penalties;
- `score`: uses the game's Type-A line awards (40/100/300/1200 × level+1), making Tetrises materially more valuable.

When the preview piece is available, the policy includes one-piece lookahead at half weight. Candidates that leave no legal preview-piece spawn receive a large top-out penalty.

`tetris/policy.Step` combines exactly one policy decision with one verified `control.Place` transaction. Long-running loops, run configuration, telemetry and replay integration remain runtime concerns rather than policy responsibilities.

## Run integration

Tetris uses the same farm lease, cancellation, save-state, recording and replay plumbing as Pokémon while keeping game-owned semantics separate.

A farm run uses:

```json
{
  "game": "tetris",
  "planner": "policy",
  "starter": "",
  "goal": "score:10000"
}
```

Supported goals are `auto`, `endless`, `survival`, `score:N`, `lines:N`, and `complete`. `endless` runs score-oriented Type A without a score target and records the final score when the board tops out; score/survival also run Type A, while lines/complete run Type B. The operator's New Run form exposes Tetris directly and fixes the planner to the deterministic policy runtime.

Farm heartbeats retain the legacy Pokémon fields for backwards compatibility and add two optional game-owned envelopes:

- `game_state`: Tetris board rows, active/next piece, mode/screen, score, lines, level and transition flags;
- `game_decision`: the latest placement target, objective, candidate count, board metrics and policy scores.

The wall persists those envelopes and clears them on a fresh lease/retry just like Pokémon player/planner telemetry. The operator live view renders the 10×18 board and Tetris state instead of presenting map/party placeholders.

Session recordings store the generic cartridge game/revision identity. The replay ROM library resolves `game=tetris` through `profiles.DetectCartridge`, after which the recording's exact ROM SHA-256 remains the final replay identity check.

## Current scope

PokePilot can now launch, observe, choose, execute, record and replay Tetris through the normal run infrastructure. Phase 6 is qualification: exercise real Rev-1 runs and establish autonomous benchmark expectations before treating the integration as production-qualified.


## Typed placement decisions (Jev)

Tetris does not need the generative Pokémon strategist. Its deterministic policy still owns the hard safety boundary: it enumerates reachable rotations/columns, simulates landing and line clears, scores the resulting board, and exposes only those legal candidates.

A fast typed-decision backend can then choose among that finite set. The first supported backend is TypeSafe Jev:

- decision kind: `tetris_placement`;
- active mode: an accepted Jev choice selects one already-legal placement;
- shadow mode: the deterministic scorer executes while Jev agreement is measured;
- low confidence, transport errors, invalid responses, or unavailable answers fall back to the deterministic scorer;
- Jev never emits buttons, coordinates outside the candidate set, or arbitrary emulator actions.

Farm runs keep `planner: "policy"`; Jev is selected independently through `decision_engine`. The operator launch form defaults Tetris to `endless` and prefers an available TypeSafe-choice deployment whose registry `default_for` includes `tetris`, then falls back to any available Jev deployment or the runner's Jev endpoint when no model registry exists. The shipped deployment registry includes the tokenless LAN target `JEV 9B · local 8077` at `http://192.168.50.80:8077/v1`.

Local `-planner policy` runs also use the runner's `POKEPILOT_DECISION_*` settings when a typed backend is enabled. For a local JEV-9B server with a 16-choice limit, pass `-max-choices 16`. The selector ranks all reachable placements with the deterministic policy, offers its best 16 to Jev, and keeps the full policy result as fallback. The limit is optional for other backends; an invalid value fails the run before play begins.

```bash
POKEPILOT_ROM=/private/Tetris.gb \
POKEPILOT_DECISION_BACKEND=jev POKEPILOT_DECISION_MODE=shadow \
POKEPILOT_DECISION_URL=http://127.0.0.1:8077/v1 \
POKEPILOT_DECISION_MODEL=jev9-local POKEPILOT_DECISION_TOKEN=local-only \
  go run ./cmd/pokepilot -planner policy -goal score:1000 -max-choices 16
```

Use `POKEPILOT_DECISION_MODE=active` to execute accepted Jev choices. Farm runs set the limit per run as `decision_engine.max_choices` (the launch form defaults it to 16; 0 offers every legal placement). Their run spec still needs `decision_engine.placements: true`.

Example run fragment:

```json
{
  "game": "tetris",
  "planner": "policy",
  "goal": "score:10000",
  "decision_engine": {
    "backend": "jev",
    "mode": "active",
    "placements": true,
    "min_confidence": 0.65,
    "max_choices": 16
  }
}
```

## Phase 6 qualification

`cmd/tetrisbench` is the ROM-backed autonomous qualification harness. It uses the same semantic state, policy candidate set, typed selector, controller, and session loop as normal Tetris runs, but does not invoke a generative LLM.

The built-in profiles are:

- `fast`: one independently booted run must reach score 1,000 within 120 pieces / 180,000 gameplay frames.
- `full`: three independently seeded runs must each reach score 10,000 within 600 pieces / 900,000 gameplay frames.

Both profiles default to active Jev with a 0.65 confidence floor and the best 16 policy candidates per decision. `--max-choices 0` offers the full candidate set to backends that accept it. The emitted `tetris-benchmark.json` records score/lines/pieces, emulator and wall time, Jev call/fallback/error counts, backend/model identity, and decision p50/p95 latency. Every required run must pass; a partial pass is a failed qualification.

Run locally on a private ROM host:

```bash
TETRIS_ROM=/private/Tetris.gb TYPESAFE_API_KEY=... \
  go run ./cmd/tetrisbench --profile fast --output /tmp/tetris-fast

TETRIS_ROM=/private/Tetris.gb TYPESAFE_API_KEY=... \
  go run ./cmd/tetrisbench --profile full --output /tmp/tetris-full
```

The private `ROM-backed Qualification` workflow exposes the same `none / fast / full` Tetris gate. It reads the ROM from the `TETRIS_ROM_PATH` repository variable and expects Jev credentials on the self-hosted runner. A clean manual `full` run closes #2015 with the workflow run as qualification evidence; public PR CI remains ROM-free.
