# RUNNOTES — S5b-4 verify: re-verified the restored Heal at f5bcbff

## What this run did (and why)
- Task text said the work was "on this branch at HEAD (000b977)".
  Stale: 000b977 (preserve of failed run e7239d7c) had been
  ORPHANED when the S5b-4 recovery decomposed off the snapshot;
  at session start the heal files were absent from HEAD b56de63.
- Mid-run, operator maestroi restored the work as f5bcbff (03:58,
  on b56de63). Its tree is byte-identical to 000b977
  (`git diff 000b977 f5bcbff` is empty), so this is the same
  verified work, re-committed. 000b977 still exists by SHA.
- Conditional restore was a no-op (tree clean); this run only
  re-verified. I committed nothing.

## Verification result (this run)
- go build/vet clean; 9/9 packages ok; 0 SKIP, 0 FAIL
  (/tmp/opencode/s5b4-verify.log).
- TestHeal PASS 0.56s: before, Squirtle 0xB1 lvl 6 HP 13/22
  (pre-assertion live, not vacuous) -> after, 22/22.
- Source facts intact: Heal at heal.go:132; TestHeal at
  heal_test.go:24 with the vacuous-branch at 56-58;
  fixtureVersion 4 (fixture.go:31).

## For the next task (S5b-5: Train, then S5b-6 toward Pewter)
- S5b-5: test from pallet_town on Route 1, keep under a minute;
  grass tile id from the tileset table world/grid.go already
  parses (offset +10), not a hardcoded tile or coordinate box.
- Fixtures are v4 and warm (all five .state files present);
  a cache miss rebuilds automatically - do not edit fixture.go.
- post_errand fixture: (19,8) on 0x01, Pokedex held, controllable.
- Route 2 exit = Viridian's north edge (row 0, x17-19); landing
  on 0x0D is (8,71). AVOID Route 22 (the rival stands there).
- Tall grass: Travel, never GoTo. Do not hand-verify whole grids.
- Heal's counterDirection needs the counter as the player's only
  solid neighbor; Pewter's nurse is adjacent with no counter, so
  face her by sprite or add an explicit approach tile if S5b-6
  or S5b-7 needs to heal there.
