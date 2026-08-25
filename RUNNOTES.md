# RUNNOTES — S3-4 (done)

## What changed
Commit "state: decode moves, PP and battle result; fix EnemyMaxHP address".

- `red/sym/addresses.go`: added `BattleResult` (0xCF0B), `BattleMonSpecies` (0xD014),
  `BattleMonMoves` (0xD01C), `BattleMonMaxHP` (0xD023), `BattleMonPP` (0xD02D),
  `EnemyMonLevel` (0xCFF3), `EnemyMonMaxHP` (0xCFF4), `PartyMon1HP` (0xD16C).
- `red/sym/addresses_test.go`: all new labels added to the pokered.sym cross-check table.
- `red/state/battle.go`:
  - BUG FIX: `EnemyMaxHP` now reads `sym.EnemyMonMaxHP` (0xCFF4), was `sym.EnemyMonHP + 2`
    (0xCFE8 — the enemy HP pair's trailing bytes, wrong). Stale comment deleted.
  - `BattleState` gained `ActiveSpecies`, `ActiveMaxHP`, `EnemyLevel`, `Moves [4]Move`.
  - New `Move{ID, PP}`; `Usable() []int` = slot indices with ID != 0 and PP > 0, slot order.
  - New `BattleResult` (ResultWon=0, ResultLost=1, ResultDraw=2) + `DecodeBattleResult(m)`.
- `red/state/battle_test.go`: synthetic-Mem tests — all 4 move slots decode with PP;
  Usable() excludes empty (ID 0) and PP-0 slots; EnemyMaxHP regression test writes
  17 to 0xCFE8 and 50 to 0xCFF4 and asserts 50 (verified it FAILS if the address is
  reverted to EnemyMonHP+2); DecodeBattleResult table test.

## Must know for next task
- `DecodeBattle` still returns nil when IsInBattle != 1/2 (contract unchanged).
- Move slots: wBattleMonMoves 0xD01C / wBattleMonPP 0xD02D, 4 bytes each, ID 0 = empty.
- wBattleResult 0xCF0B: 0 won / 1 lost / 2 draw (matches pokered BATTLE_WON/LOST/DREW).
- wPartyMon1HP 0xD16C is PartyMon1+1, handy for post-battle party HP checks.
- No battle skill implemented (out of scope here); skill/move.go already probes
  DecodeBattle for in-battle detection.
- skill/, world/, emu/ untouched. TestGoToViridianPokecenter still red by design;
  verify with `-skip TestGoToViridianPokecenter`.
- ROM: /home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb
