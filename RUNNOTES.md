# RUNNOTES — task 10: S5b-4 handoff note already absent

## What happened
- .agent-runner/HANDOFF.md did not exist here: the worktree was
  reset to base (289fa57) before this task started, taking the
  note with it. Nothing to delete; the tree was clean at start.
- No S5b-4 implementation work was done or needed from this task.

## CRITICAL for the next task (S5b-4 verification)
The S5b-4 work is NOT on this branch. HEAD is 289fa57 and
skill/heal.go / skill/heal_test.go do not exist in it. The work
survives as dangling commit 000b977, whose parent is exactly
the current tip:
  000b977: skill/heal.go (188 lines; func Heal at line 132),
  skill/heal_test.go (89 lines; func TestHeal at line 24),
  RUNNOTES S5b-4 section (Squirtle 13/22 HP, TestHeal 554ms).
Inspect with `git show 000b977 --stat`.
Do NOT run `git checkout HEAD -- skill/heal.go`: the path is not
in HEAD and the command will error. A missing file is not a
modified one — no restore can conjure it. If the heal files are
missing, STOP and report: the fix is a one-step fast-forward of
this branch to 000b977, then re-run the verification.

## Carried from the S5b-3b note (for S5b-5/S5b-6/S5b-7)
- post_errand fixture: (19,8) on 0x01, Pokedex held, no parcel,
  controllable. fixtureVersion = 4; bump only if the validity
  definition of a state changes again.
- Route 2 exit = Viridian's north edge (row 0, x17-19); landing
  on 0x0D is (8,71), Place("route 2").
- Do NOT route through Route 22 (rival stands there; a forced
  battle aborts Travel). The city's west edge is the R22 edge.
- Tall grass: Travel, never GoTo (GoTo aborts on wild battles by
  design). Route 2 is 20x72, the forest 34x48 — do not
  hand-verify whole grids; that burned three S5-3 attempts.
- BrockData `db $FF, 12, GEODUDE, 14, ONIX, 0`. Fixture Squirtle
  learns BUBBLE at 8 and walks both. Do not tune the move policy
  for Charmander — unwinnable there, documented, not our problem.
