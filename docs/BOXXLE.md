# Boxxle support

PokePilot supports **Boxxle (USA, Europe) (Rev 1)** as its second non-Pokémon Game Boy title.

Supported fingerprint:

- SHA-1: `f9d5287bc9d6eda9ec36e9b5a8dbc38cf2cffecf`
- SHA-256: `c859503342db1f86dadeb7e6f3d8a8a2918e9b6a7c8756311b7cc7bb0a7e892f`

The bytes themselves are operator-supplied and never committed.

## Runtime boundary

Boxxle uses `game.CartridgeProfile` for ROM identity and exposes semantic gameplay state through `boxxle.StateProfile`. It does not implement Pokémon's `game.GameProfile`.

`boxxle.State` exposes the current screen, board dimensions, walls, goals, crates, player cell, and whether every crate sits on a goal. Revision-owned raw addresses live in `boxxle/sym`. The layout was measured on the real cartridge:

- The level is the 32×32 background map (`0x9800`, or `0x9C00` when LCDC bit 3 is set) in 2×2-tile cells.
- A cell's kind is its top-left tile id: wall `A8`, crate `A4`, goal `A0`, crate on goal `AC`.
- Floor and the exterior share one tile id (`D4`), so the playable interior is recovered by flood fill from the player.
- The player is OAM sprite 0. A crate in motion is also a sprite.

`boxxle.RenderBoard` is the compact ASCII board used by planners, heartbeats, and the operator/spectator UI (`#` wall, `.` floor, `@` player, `$` crate, `*` crate on goal, `+` goal).

## Deterministic execution

`boxxle/control.Push` is the closed-loop primitive. A target is one legal crate push. The executor walks the player to the pre-push square, taps the D-pad once per cell, and verifies every step against the decoded board. Invalid pushes are rejected before input.

`boxxle/policy` enumerates legal, non-deadlocking pushes. `boxxle/solver` is the ROM-free A* oracle over push states. The default session selector solves the live board once and replays that plan, re-solving if a planned push is no longer legal.

## Run integration

A farm run uses:

```json
{
  "game": "boxxle",
  "planner": "policy",
  "starter": "",
  "goal": "first"
}
```

Supported goals:

- `first` (default) — solve the first puzzle and advance;
- `early` — the first-room qualification batch (`session.EarlyLevels`, currently 5);
- `levels:N` — solve N puzzles;
- `endless` — keep going until frame/push budget or cancel.

`planner: launch` only boots and registers the cartridge.

Farm heartbeats publish a Boxxle-owned envelope:

- `game_state`: kind, screen, ASCII board, crates on goal, pushes, puzzles solved, player cell;
- `game_decision`: the latest semantic push, fallback flag, and optional typed-backend identity.

Replay recordings store the generic cartridge game/revision identity. The replay ROM library resolves `game=boxxle` through `profiles.DetectCartridge`.

## Typed push decisions

A configurable typed-decision backend may choose only from already-legal, non-deadlocking pushes. The model never emits raw D-pad input. Low confidence, transport errors, or invalid answers fall back to the deterministic solver and are recorded as fallbacks.

```json
{
  "game": "boxxle",
  "planner": "policy",
  "goal": "early",
  "decision_engine": {
    "backend": "jev",
    "mode": "active",
    "placements": true,
    "min_confidence": 0.65,
    "max_choices": 16
  }
}
```

## Qualification

ROM-gated session tests (`BOXXLE_ROM`, default `roms/boxxle.gb`, skipped without it) prove the first milestone and the early batch:

```text
boot ROM
  -> enter puzzle gameplay
  -> decode board
  -> solve autonomously
  -> advance to the next puzzle
```

`TestRunSolvesAndAdvances` solves two puzzles. `TestEarlyLevelBatch` solves `EarlyLevels` puzzles. Both report whether solver fallback was used.

`cmd/boxxlebench` is the ROM-free solver/oracle harness over the checked-in fixtures. It compares a planner against the deterministic baseline without a cartridge.
