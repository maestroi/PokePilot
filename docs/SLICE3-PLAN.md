# Slice 3 draft plan — story progression and battle

Status: **drafted, not measured, not published.**

Goal: boot a fresh game and reach the Viridian Pokémon Center nurse. That is
slice 2's goal, and slice 2 cannot reach it, because Red will not leave Pallet
Town without a Pokémon. Slice 3 is the work that makes slice 2's milestone
test pass.

Prerequisite: slice 2, merged at `3d03cfd`. Full suite green except
`TestGoToViridianPokecenter`, which fails at exactly the gate described below.

---

## Why slice 2 stalls

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

`PAD_SELECT|PAD_START|PAD_CTRL_PAD` is `0xFC`. That is the byte the S2-7 worker
measured appearing at `wJoyIgnore` (0xCD6B) one frame after the player reaches
`y == 1`, freezing movement in all four directions. It is Oak's "Hey! Wait!
Don't go out!" cutscene, not an emulator defect. `TestGoToViridianPokecenter`
fails with `step up blocked at (10,1)` — `y == 1`, as the script says.

The S2-7 notes conclude the opposite ("This is a gomeboy/game behavior... A fix
means changing the gomeboy emulator"). That conclusion is wrong and would have
cost days in the wrong repository. Recorded here so it is not repeated.

---

## Ground truth

**Everything in this section is read from the pokered decomposition, not yet
driven on the ROM.** That is the opposite of how slice 2's ground truth was
established, and slice 2 worked precisely because every value in its task text
had been measured first. S3-0 exists to close that gap; nothing below should be
trusted in task text until it has.

### Event flags (`wEventFlags` = 0xD747, bit N at byte `0xD747 + N/8`, bit `N%8`)

Derived by counting `const`/`const_skip` in `constants/event_constants.asm`:

| Event | Bit | Byte | Bit in byte |
|---|---|---|---|
| `EVENT_FOLLOWED_OAK_INTO_LAB` | 0 | 0xD747 | 0 |
| `EVENT_OAK_ASKED_TO_CHOOSE_MON` | 33 | 0xD74B | 1 |
| `EVENT_GOT_STARTER` | 34 | 0xD74B | 2 |
| `EVENT_BATTLED_RIVAL_IN_OAKS_LAB` | 35 | 0xD74B | 3 |
| `EVENT_GOT_POKEBALLS_FROM_OAK` | 36 | 0xD74B | 4 |
| `EVENT_OAK_APPEARED_IN_PALLET` | 38 | 0xD74B | 6 |

Counted, not measured. Every one of these is self-verifying: drive the step
that sets it, then assert the bit flipped. Do that before writing it into a
task.

### Oak's Lab — map 0x28, from `data/maps/objects/OaksLab.asm`

```
warp_event   4, 11, LAST_MAP, 3
warp_event   5, 11, LAST_MAP, 3
object_event 4,  3, SPRITE_BLUE, ..., OPP_RIVAL1, 1
object_event 6,  3, SPRITE_POKE_BALL, ..., TEXT_OAKSLAB_CHARMANDER_POKE_BALL
object_event 7,  3, SPRITE_POKE_BALL, ..., TEXT_OAKSLAB_SQUIRTLE_POKE_BALL
object_event 8,  3, SPRITE_POKE_BALL, ..., TEXT_OAKSLAB_BULBASAUR_POKE_BALL
object_event 5,  2, SPRITE_OAK, STAY, DOWN, TEXT_OAKSLAB_OAK1
```

The starter is **not** a menu choice. You walk to the table, face a ball, and
press A; the ball's text runs `YesNoChoice` (`scripts/OaksLab.asm:896`), which
is an ordinary two-item menu — `wCurrentMenuItem` 0 = YES, 1 = NO. So the
starter is chosen by *position*, and confirmed by *cursor*.

The rival battle is not optional and is not triggered by talking. From
`OaksLabRivalChallengesPlayerScript`: it fires on `wYCoord == 6`, i.e. as soon
as you walk back toward the exit. Any route out of the lab walks into it.

### RAM addresses (all confirmed present in `pokered.sym`)

| Symbol | Address | Note |
|---|---|---|
| `wCurrentMenuItem` | 0xCC26 | already in `red/sym` |
| `wMaxMenuItem` | 0xCC28 | already in `red/sym` |
| `wBattleMonSpecies` | 0xD014 | start of the BattleMon struct |
| `wBattleMonHP` | 0xD015 | already in `red/sym` |
| `wBattleMonMoves` | 0xD01C | 4 bytes |
| `wBattleMonLevel` | 0xD022 | already in `red/sym` |
| `wBattleMonMaxHP` | 0xD023 | |
| `wBattleMonPP` | 0xD02D | 4 bytes |
| `wEnemyMonMaxHP` | 0xCFF4 | |
| `wEnemyMonLevel` | 0xCFF3 | |
| `wBattleResult` | 0xCF0B | win/lose/draw |
| `wPartyCount` | 0xD163 | |
| `wPartyMon1HP` | 0xD16C | |
| `wJoyIgnore` | 0xCD6B | the gate byte |
| `wPalletTownCurScript` | 0xD5F1 | |

---

## Decisions

- **`Cutscene` does not fight the game.** Every skill so far drives input until
  a RAM predicate holds. During a scripted sequence the game drives and the
  skill must stop pressing directions — pressing into `wJoyIgnore` is exactly
  what the S2-7 worker did before concluding the emulator was broken. The
  termination condition is an event flag, never a frame count.
- **Menus are step-and-verify, same shape as movement.** Press Up/Down, assert
  `wCurrentMenuItem` reached the wanted index, then A. No press counts, no
  pixels. This is what keeps the LLM out of menu navigation permanently.
- **`Battle` takes a `MovePolicy func(BattleState) int`.** The default is
  deterministic — highest-power damaging move with PP remaining — so the tests
  never call a model. That seam is where the LLM eventually plugs in, and it is
  the first real strategy decision in the project; everything before it was
  mechanism.
- **The starter is a parameter, not a constant.** `GetStarter(m, rom, which)`.
  The test uses Squirtle at (7,3); nothing in the code hard-codes it.
- **Blacking out is a real outcome, not a test failure.** `Battle` returns a
  typed result. Recovery from a blackout is out of scope for this slice.

---

## Tasks

Published to agent-runner as plan `64867bf8-7ad1-4cd7-888b-7ee327f2c12f`
(**draft** — see Process notes before publishing).

| | Task | First edit | Depends |
|---|---|---|---|
| S3-1 | Story event flags — read `wEventFlags` by name | `red/state/progress.go` | — |
| S3-2 | `Cutscene` — endure scripted control loss | `skill/cutscene.go` | S3-1 |
| S3-3 | Menu state + `SelectMenuItem` cursor loop | `red/state/menu.go` | — |
| S3-4 | Battle state: moves, PP, max HP, result | `red/state/battle.go` | — |
| S3-5 | `Battle` — fight to a decision, pluggable policy | `skill/battle.go` | S3-3, S3-4 |
| S3-6 | `GetStarter` — the composite that opens the gate | `skill/story.go` | S3-2, S3-5 |
| S3-7 | Close slice 2: milestone test + the S2-7 fixtures | `skill/fixture/fixture.go` | S3-6 |

The probe that measures this slice's ground truth is not a task. Slice 2's
measurement was done by hand before any task text was written, and that is why
S2-0 through S2-5 landed clean on first attempt. Handing measurement to a
worker is how the "gomeboy emulator bug" conclusion happened.

Tasks S3-1 through S3-6 verify with `-skip TestGoToViridianPokecenter`, because
that test stays red until S3-7 by design. S3-7 runs the full suite with nothing
skipped — that is how the slice proves it closed slice 2.

S3-7 is where slice 2 finally closes. `TestGoToViridianPokecenter` starts
passing without `skill/goto.go` changing at all, and the three fixtures S2-7
never built (`pallet_town`, `viridian_city`, `viridian_pokecenter`) become
producible, because the route out of Pallet Town is finally open.

---

## Process notes

Two things went wrong in slice 2 that this plan has to account for.

- **A failing test was reported `done`.** S2-6 and S2-7 both came back
  `status: "done"` with `TestGoToViridianPokecenter` red. S2-6's `result_sha` is
  S2-5's commit — it produced no commit at all — and S2-7 never touched
  `fixtureVersion`, which was its entire task. Whatever marks a task complete is
  not the verification command's exit code. Worth fixing before eight more tasks
  are queued, or slice 3 reports green the same way.
- **A worker diagnosed a game script as an emulator bug** and wrote it into
  `RUNNOTES.md` as a conclusion, with a recommended fix in the wrong repository.
  Task text should say plainly: if input stops working, read `wJoyIgnore` and
  find the script that set it before blaming anything below the game.

And the rule that held: measure before writing task text. It is why S2-0 through
S2-5 landed clean on first attempt. This document is not measured yet.
