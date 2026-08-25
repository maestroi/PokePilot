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

---

## Addendum — S3-6 measured, 2026-08-25

S3-6 failed twice. Neither failure was a test failure: both agent-runner runs hit
the 45-minute runtime cap (`runtime stalled or exceeded max runtime`) with no
output captured, peaking at 107k/118k of a 131k context. Attempt 1 wrote a
complete `skill/story.go` and never got to commit it; attempt 2 discarded that and
started writing probe programs.

Running attempt 1's code by hand takes **2.6 seconds** and fails cleanly:

    GetStarter returned  map=0x28 (5,3) ctrl=true followedOak=true
      GoTo below the ball: no path on map 28 from (5,3) to (7,4): world: no path

The task was not too slow. It was built on four wrong facts, three of which this
plan asserted as ground truth and told the worker not to re-derive.

### 1. `world.Build` samples the wrong tile of each block

A block is 4x4 tiles; a game coordinate ("step") is 2x2 tiles. `world/grid.go`
took the step's **top-left** tile:

    tile := romData[tilesOff+(2*sy)*4+2*sx]

The game's collision uses the tile at the player's feet — the step's
**bottom-left**, `2*sy+1`. The effect is an extra blocked row beneath every tall
object, on every map. In Oak's Lab it makes (6..8, 4) — the tile this plan tells
you to stand on — unwalkable, so `FindPath` correctly reports no path to a
destination that is fine in the real game.

Measured by walking the lab exhaustively with save states (`o` = the game let the
player stand there, `S` = where the entry cutscene leaves them):

          0123456789          Build, top-left      Build, bottom-left
       0  ##########          ##########           ##########
       1  ####oo####          ####..####           ####..####
       2  ooooo#oooo          ####......           ..........
       3  oooo#S###o          ......###.           ......###.
       4  oooooooooo          ......###.           ..........
       5  oooooooooo          ..........           ..........

With `2*sy+1` the grid matches the walked result exactly. The two remaining
differences are sprites, which `Grid` does not model: Oak stands at (5,2) and the
rival at (4,3).

This also confirms the decomp, which asks `cp 4 ; is the player standing below the
table?` — y == 4 is a real standing position.

### 2. `EVENT_FOLLOWED_OAK_INTO_LAB` does not mean the player may move

`OaksLabFollowedOakScript` sets it, and the very next script re-seizes control:

    OaksLabOakChooseMonSpeechScript:
        ld a, PAD_SELECT | PAD_START | PAD_CTRL_PAD
        ld [wJoyIgnore], a
        ... four text boxes ...
        SetEvent EVENT_OAK_ASKED_TO_CHOOSE_MON
        xor a
        ld [wJoyIgnore], a

Measured: at `EVENT_FOLLOWED_OAK_INTO_LAB` the player is at (5,3) with
`joyIgnore=0`, `font=0`, `Controllable=true`, and **all four directions blocked**.
The predicate that means "you may move" is `EVENT_OAK_ASKED_TO_CHOOSE_MON`.

### 3. `EVENT_GOT_STARTER` is not set when you take the ball

Taking the ball sets `BIT_GOT_STARTER` in `wStatusFlags4` (0xD72E, bit 3) and adds
the party mon. `EVENT_GOT_STARTER` is set later, in
`OaksLabRivalChoosesStarterScript`, after the rival has taken his. Asserting it
immediately after the yes/no menu is too early.

### 4. `wYCoord == 6` has two meanings in the lab

`OaksLabPlayerDontGoAwayScript` also fires on y == 6 and force-walks the player
back up. Which script runs depends on `wOaksLabCurScript`. The rival challenge on
y == 6 is only reachable once `EVENT_GOT_STARTER` is set.

### 5. S3-1's bit index for `EVENT_OAK_APPEARED_IN_PALLET` is wrong

This plan's derived table omitted two constants. Recounted from
`constants/event_constants.asm`:

    EVENT_FOLLOWED_OAK_INTO_LAB_2          32
    EVENT_OAK_ASKED_TO_CHOOSE_MON          33
    EVENT_GOT_STARTER                      34
    EVENT_BATTLED_RIVAL_IN_OAKS_LAB        35
    EVENT_GOT_POKEBALLS_FROM_OAK           36
    EVENT_GOT_POKEDEX                      37
    EVENT_PALLET_AFTER_GETTING_POKEBALLS_2 38
    EVENT_OAK_APPEARED_IN_PALLET           39

`red/state/progress.go` shipped 38. The five indices S3-2..S3-5 actually exercise
are correct.

### What this changes about writing tasks

The instruction "do not re-derive, this is ground truth" left the worker no
sanctioned move when reality disagreed. It spent 45 minutes not allowed to
conclude the plan was wrong. Ground truth stated in a task must be labelled with
how it was established — read from the decomp, or measured on the ROM — and a task
must always be permitted to report that a stated fact is false.

---

## Addendum 2 — the rival battle, measured 2026-08-25

R3 reached the rival battle and lost it every time. The reported cause was the
rival's counter-pick. That is real — take Squirtle and he takes Bulbasaur — but
it is not the cause: at level 5 Squirtle knows TACKLE/TAIL_WHIP and Bulbasaur
knows TACKLE/GROWL. Neither has a Water or Grass move, so type matching cannot
apply. Two other things were wrong.

### 1. wFontLoaded is never set during a battle

`skill/battle.go` gated `mainMenuUp` and `moveMenuUp` on `wFontLoaded != 0`.
MEASURED: `wFontLoaded` stays 0 for the entire battle — battle text does not go
through the overworld text engine. So neither predicate was ever true across a
whole fight, the `MovePolicy` was never called once, and `Battle` fell through
to its "advance a text box" fallback and mashed A from start to finish.

`wMaxMenuItem` cannot replace it either: it keeps the move menu's value
(`wNumMovesMinusOne + 2`) while the "used TACKLE!" text that follows is on
screen. The menus are now identified from `wTileMap` instead, which is RAM and
already decoded for the dialogue trace:

    main menu   wMaxMenuItem 1, cursor 0, screen contains "FIGHT"
    move menu   wMaxMenuItem numMoves+1, cursor 1..numMoves, screen has "TYPE/"

The move menu really is 1-indexed, as `MoveSelectionMenu` stores
`wPlayerMoveListIndex + 1` into `wCurrentMenuItem`. S3-5 had that right.

### 2. Always attacking loses on purpose, not by chance

The rival's Bulbasaur opens with GROWL — four times in one observed fight. Each
one costs a stage of Attack, and `FirstUsableMove` has no reply:

    Enemy BULBASAUR used GROWL!  x4
    player Tackle damage:  3 -> 2 -> 2 -> 2 -> 1 -> 1
    enemy  Tackle damage:  3 -> 3 -> 3

Bulbasaur is also faster (45 to 43), so even an undisturbed Tackle race is lost
by a turn. The answer is a Defense-lowering move: Gen 1 damage scales on the
ratio of Attack stage to Defense stage, so -1 to theirs cancels -1 from ours
exactly. `skill.StatAwareMove` reads `wPlayerMonAttackMod` (0xCD1A) and
`wEnemyMonDefenseMod` (0xCD2F) and spends a turn on TAIL WHIP while behind and
above half HP, otherwise hits with the highest-power move it has. Move power
and effect come from the ROM's move table (bank 0x0E:0x4000, six bytes an
entry), so nothing hardcodes a move id.

Lowering the opponent's ATTACK was tried and removed: it cost Charmander three
extra turns and did not save Bulbasaur, because **Gen 1 critical hits ignore
stat stages entirely** — a -3 Attack Charmander still took Bulbasaur from 7 HP
to 1 in one turn.

### Result, and one honest limitation

Deterministic across repeated runs:

    charmander   won in 6 policy turns
    squirtle     won in 9 policy turns
    bulbasaur    LOSES

Bulbasaur losing is a matchup, not a policy bug. Its TACKLE (35) into
Charmander's Defense 43 does about 3; Charmander's SCRATCH (40) into Defense 49
does about 4; and Charmander is much faster (65 to 45). With only TACKLE and
GROWL there is no line that wins. Fixtures use Squirtle. If the slice 4
random-starter experiment ever rolls Bulbasaur, it will lose the lab battle
until the policy gets something better than a stage heuristic — that is a real
finding about the game, worth keeping rather than tuning away.

### Correction — losing the rival battle is survivable

Addendum 2 above assumed a lost rival battle meant a blackout and an
unrecoverable run. MEASURED, and wrong. Losing it leaves:

    map=0x28 at (5,6)  controllable=true  party=1
    followedOak=true   gotStarter=true    rival=true
    Route 1 (map 0x0C) reachable

There is no blackout. `OaksLabRivalEndBattleScript` runs whichever way the
battle went: it calls `predef HealParty` and sets
EVENT_BATTLED_RIVAL_IN_OAKS_LAB unconditionally.

More to the point, the north exit was never gated on the battle at all.
`PalletTownDefaultScript` opens with

    CheckEvent EVENT_FOLLOWED_OAK_INTO_LAB
    ret nz

so the gate lifts the moment Oak walks the player into the lab — before the
starter is taken, and long before the rival fight.

So GetStarter must NOT require ResultWon. Its postcondition is
EVENT_BATTLED_RIVAL_IN_OAKS_LAB set, the player controllable, and a party of
at least one — all of which hold after a loss. `StatAwareMove` stays the
default because winning is still better (the starter keeps the experience),
but a loss is a result, not an error.

This also un-blocks the slice 4 random-starter experiment: Bulbasaur cannot
win this fight, and now it does not have to.
