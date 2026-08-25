# RUNNOTES — S3-5 (done)

## What changed
Commit "skill: fight a battle to a decision with a pluggable move policy".

- `skill/battle.go` (new):
  - `MovePolicy func(state.BattleState) int` — the seam for a learned policy.
  - `FirstUsableMove` default policy: lowest slot with ID != 0 and PP > 0, else -1.
  - `ErrNoUsableMove`; `Battle(m, policy) (state.BattleResult, error)`.
  - State machine: main menu (`FontLoaded != 0 && MaxMenuItem == 1`) → `SelectMenuItem(m, 0)`
    (FIGHT) → wait for move menu (`MaxMenuItem >= 2`) → `SelectMenuItem(m, slot+1)`
    (move menu is 1-indexed: slot i sits at cursor i+1) → wait for `FontLoaded == 0`
    (menu closed) → loop. Anything else (text box/animation, stale wMaxMenuItem) → Tap A.
  - Battle ends when `DecodeBattle == nil` (IsInBattle 0): `settleAfterBattle` advances
    end text (Tap A while `FontLoaded != 0`), waits for `Controllable`, returns
    `DecodeBattleResult`. Losing = ResultLost, nil error.
  - Frame budgets: 60000 total (cap trips → loud error), 500 for move-menu appear/close,
    3000 for post-battle settle. Every error carries map, coords, decoded battle state.
  - No item/switch handling: party menu after a faint is not handled → frame cap fails loudly.
- `skill/battle_test.go` (new): `TestFirstUsableMove` (unit, 4 cases, no ROM);
  `TestBattleNoBattleInProgress` (ROM-gated: error + player controllable + coords unchanged).

## Must know for next task (S3-6 rival fight)
- `Battle` is the only new skill; drive the trainer encounter first (GoTo/Talk/Cutscene),
  then call `Battle(e, skill.FirstUsableMove)` (or any MovePolicy).
- Main-battle-menu discriminator is `wMaxMenuItem == 1`; move menu `>= 2`. Text boxes carry
  a stale wMaxMenuItem — never use it alone.
- Move menu cursor is 1-indexed; `SelectMenuItem(m, i+1)` presses move slot i.
- `wIsInBattle` (0xD057) clears in EndOfBattle AFTER the win/EXP text; `wBattleResult`
  (0xCF0B) is set before that. Don't read the result until IsInBattle == 0.
- If the player's mon faints, Battle does NOT switch: it will hit the frame cap and fail
  loudly. S3-6's rival fight must use a mon that wins (or extend Battle — not done here).
- skill/, world/, emu/, red/ untouched. TestGoToViridianPokecenter still red by design;
  verify with `-skip TestGoToViridianPokecenter`.
- ROM: /home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb
  (also works: /home/maestro/Downloads/Pokemon - Red Version (USA, Europe) (SGB Enhanced).gb)
