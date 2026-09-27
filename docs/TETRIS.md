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

## Current scope

This layer executes an already-selected placement. It deliberately does **not** choose placements or score board positions. Search/evaluation policy belongs to the next Tetris phase so controller mechanics stay deterministic and independently testable.
