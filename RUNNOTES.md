# RUNNOTES — S2-7 (CORRECTED: not an emulator bug, a story gate)

## Correction (2026-08-25)

The original version of this file concluded that the Pallet Town freeze was
"a gomeboy/game behavior... A fix means changing the gomeboy emulator
(separate repo)", and listed "Fix gomeboy" as the first recommended next step.

**That conclusion was wrong.** There is no emulator bug. GomeBoy ran the ROM
correctly. The freeze is Pokémon Red's own script, doing exactly what it was
written to do: Red will not leave Pallet Town before he has a Pokémon.

`scripts/PalletTown.asm`, `PalletTownDefaultScript`:

```asm
	CheckEvent EVENT_FOLLOWED_OAK_INTO_LAB
	ret nz
	ld a, [wYCoord]
	cp 1 ; is player near north exit?
	ret nz
	...
	ld a, PAD_SELECT | PAD_START | PAD_CTRL_PAD
	ld [wJoyIgnore], a
```

`PAD_SELECT|PAD_START|PAD_CTRL_PAD` is `0xFC` — the exact byte measured below.
The trigger is `wYCoord == 1`, which is why it reproduced at (11,1) and (10,1)
and why the step into row 0 never happened. It is Oak's "Hey! Wait! Don't go
out!" cutscene, gated on `EVENT_FOLLOWED_OAK_INTO_LAB`.

The measurements in this file were good. Only the diagnosis was wrong.

Two things that would have caught it:

- **"The write to 0xCD6B is indirect (no `ld [0xCD6B],a` in the ROM)"** — the
  instruction is right there in the decomp as `ld [wJoyIgnore], a`. Searching
  raw ROM bytes for a hardcoded address finds nothing, because the assembler
  emits the symbol. Grep `~/.cache/pokered/`, not the ROM image.
- **Reaching for the emulator is almost always the wrong move.** GomeBoy is a
  well-tested Game Boy implementation; PokePilot's understanding of Pokémon Red
  is the young, unproven half. When input stops working, read `wJoyIgnore`
  (0xCD6B) and find the script that set it before suspecting anything below the
  game.

Resolution: slice 3 (`docs/SLICE3-PLAN.md`, agent-runner plan
`64867bf8-7ad1-4cd7-888b-7ee327f2c12f`) drives the story past the gate. Nothing
in `skill/goto.go` needs to change — the route was always correct.

---

## Original status (kept for the record)

BLOCKED. `pallet_town` fixture is producible (arrival state, no crossing).
`viridian_city` + `viridian_pokecenter` require crossing Pallet Town's north
edge (map 0x00 -> Route 1 0x0C), which hangs. S2-6 GoTo work is ported,
builds, vets, and all skill tests pass except `TestGoToViridianPokecenter`.

Also, as found later: S2-7 was marked done without doing its task.
`skill/fixture/fixture.go` still has `fixtureVersion = 2` and `reds_bedroom` as
its only registered fixture. `scratch2/main.go` was committed despite the note
below saying it was deleted; it was removed in `3d03cfd`.

## Measurements (accurate — reproduced)

Stepping the player UP from row y=1 of Pallet Town makes the game set
`wJoyIgnore` (0xCD6B) to `0xFC` within 1 frame, which freezes the player in
ALL 4 directions, persistently. Reproduced at (11,1) and (10,1).

- No map transition is in progress: `wFadeoutMode` (0xD838) = 0x00,
  `wMapStatus` (0xD828) = 0x00, map stays 0x00, player never moves.
- Map dims are correct: `wCurMapWidth`=10, `wCurMapHeight`=9 (blocks).
- Target tile (11,0)/(10,0) = 0x52, which IS in the tileset-0 walkable list.
- The player CAN step (11,2)->(11,1); only the step out of row y=1 is blocked.

All of this is consistent with the script above: the collision grid is right,
the tile is walkable, and the game is simply refusing input.

## Repro
Boot -> walk to Pallet Town (5,6) -> WalkPath to (11,2) -> StepOnce Up to
(11,1) -> Press Up. wJoyIgnore goes 0x00 -> 0xFC at frame 1, player frozen.

## Verification done
`go build ./...`, `go vet ./...` clean. `go test -count=1 ./...` green except
`TestGoToViridianPokecenter`. ROM:
/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb.
