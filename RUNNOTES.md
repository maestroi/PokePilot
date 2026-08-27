# RUNNOTES — S5b-3: delete the S5b-3b recovery handoff note

## What changed
Nothing on disk. The task asked to delete .agent-runner/HANDOFF.md, but the
file (and the whole .agent-runner/ directory) is already absent from this
worktree; the S5b-3b run that generated it was preserved as commit f17bae0,
and its evidence (gate-crossing test, post_errand fixture, fixtureVersion
bump) is committed as a69ec47. The note's own header says to remove the
file before verification or commit — that is already the state.
`test ! -e .agent-runner/HANDOFF.md && git status --short` passes as-is:
no file, no modified entries.

## Why no commit
Deleting an untracked file leaves nothing for git to record. There was
no other file to touch, per the task scope.

## For the next task (cross Route 2 to Pewter, then the gym)
- Load fixture "post_errand": (19,8) on 0x01 (Viridian), Pokedex held, no
  parcel, controllable. fixtureVersion is 4; bump only if the definition
  of a valid state changes again.
- The Route 2 exit is the city's NORTH edge (row 0, x17-19); the landing
  on Route 2 (0x0D) is (8,71), Place("route 2").
- Do NOT route through Route 22 (rival stands there; the forced battle
  aborts Travel) — the city's WEST edge is the R22 edge.
- Tall grass: use Travel, never GoTo (GoTo aborts on wild battles by
  design). Route 2 is 20x72; the Viridian Forest 34x48 — do not try to
  hand-verify whole grids, that burned three S5-3 attempts.
- BrockData: `db $FF, 12, GEODUDE, 14, ONIX, 0`. Squirtle (fixtures)
  learns BUBBLE at 8 and walks through both. Do not tune the move
  policy for Charmander — unwinnable there, documented, not our problem.
