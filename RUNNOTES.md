# RUNNOTES — S5b-5: skill.Train — grind wild battles in grass until a level target

## What landed
- skill/train.go: `Train(m, romData, targetLevel, policy, maxBattles) (TrainResult, error)`.
  Ping-pongs the player between two adjacent walkable grass cells on the
  current map; each leg is a Travel, so a mid-leg encounter is fought
  (skill.Battle + policy) and Travel re-plans the rest of the leg. The
  lead's level is re-read from RAM after every leg. The session ends on
  level >= target, budget exhausted, or blackout. Level is checked per
  LEG, so a session that hits target mid-leg may fight 1-2 more battles
  that leg (and possibly blackout) before stopping: the result can report
  both Reached and BlackedOut.
- Grass cells come from the ROM (tileset table 03:47BE, grass tile at
  +10, walkable cells only) — no coordinates, no hardcoded tiles.
- skill/train_test.go: two tests, both green, zero skips.

## Measured (real ROM, 2026-08-27, pallet_town fixture)
- Approach Travel onto Route 1 (0x0C): 0 battles.
- TestTrainGrindsOnRoute1 (target L+2, budget 12): 7 battles, L6->L8,
  ended in a blackout on battle 7, player back at (16,6) on 0x0C —
   8.61s wall. Deterministic: three runs gave identical values.
- TestTrainBudgetIsAResult (target L+8, budget 1): 1 battle, L6->6,
   nil error, no battle left — 1.58s wall.
- Route 1 grass rate 25/256: ~1 battle per 6 legs / 24 steps; the grace
  counter (wNumberOfNoRandomBattleStepsLeft) slows encounters right after
  one. Pallet Town grass has no wild entry in the ROM: zero encounters.
- Blackout with a one-mon party: this ROM did NOT visibly transport to a
  center — the next wild battle started ~860 frames later on the same
  grass and the fainted (0 HP) lead was even sent out and won (XP and
  the level-up still applied). Blackout is a legitimate session ending,
  not a failure; the test accepts it. S5b-1's Travel settle (wait for the
  wCurMap flip or a stable position, then re-plan from the world) covers
  the aftermath either way.
- The fixture's L6 Squirtle already carries Water Gun (0x21) and Bubble
  (0x27); S5b-7's "learns BUBBLE at level 8" does not match this
   fixture, so measure the Brock-fight level requirement rather than
   assuming it. XP curve (medium-slow): L6=179, L7=236, L8=314, so
   6->7 needs ~2-3 wins and 7->8 ~3-4.
- Re-verified 2026-08-27: full suite 9 packages ok, zero skips, skill
  package 53.05s.

## For the next task (S5b-6: survive trainer ambushes, reach Pewter)
- Measure FIRST whether Travel already copes with an ambush (trainer
  battles set wIsInBattle=2 and wJoyIgnore while the trainer walks
  over). After any battle ends, wIsInBattle can briefly read 0xff (stale);
  DecodeBattle returns nil then — do not treat 0xff as an error state.
- A raw manual step loop hit a dynamic obstacle (an NPC on Route 1
  blocked the step at (12,6)); Travel's retry/detour handles it. For
  diagnostics use Travel, or mirror its retries, not bare StepOnce.
- Route per S5b-6: Route 2 (0x0D) -> gate -> Viridian Forest -> gate ->
  Route 2 north -> Pewter. Earlier note: Route 2's exit is Viridian's
  north edge (row 0, x17-19), landing on 0x0D at (8,71).
- Fixtures are v4 and warm; if you add a pewter_city checkpoint, bump
  fixtureVersion to 5 (a stale cache poisons itself).
