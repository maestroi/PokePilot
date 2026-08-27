# RUNNOTES — S5c-6: slice 5 close-out, and why the badge is not proven

## Outcome

Slice 5's five code tasks landed. The sixth, proving the Boulder Badge three
runs running, **did not** — and that is the recorded result, not a pending
item. `TestGymBoulderBadge` is skipped with its reason; slice 6 owns the proof.

## What landed

- **S5c-2** `ErrReplanExhausted` — re-plan exhaustion is now distinguishable
  from an ordinary unwalkable leg, while keeping both identities in the chain.
- **S5c-3** `Traverse` picks a warp tile the pathfinder can actually reach, so
  the forest gates cross on ordinary legs. `crossGate` is deleted from the test.
- **S5c-4** `state.DecodeSprites` — live NPC positions from `wSpriteStateData1`
  liveness plus `wSpriteStateData2` coordinates.
- **S5c-5a** `walkAround` re-reads blockers from fresh RAM every attempt; the
  collision-memory maps are gone.
- **S5c-5b** every path planner (`goto.go`, `warp.go`, `story.go`) plans on the
  live blocker overlay; `walkLab`'s static ball-tile exclusions are merged in,
  never mutated.
- **S5c-6** the diagnostic bundle (`diagFatalf`), and one real fix found while
  measuring: `Traverse` now types an unreachable connection edge as
  `ErrLegUnwalkable`. Unwrapped it was terminal, which killed the only real
  route to Pewter — Route 2's ledge makes the north edge unreachable from the
  southern landing tile, and GoTo's per-tile ban is what re-routes through the
  forest.

## Why the badge is not proven

Measured with all close-out fixes in place:

    trained the lead to level 12 in 18 battles          ~1 minute
    text box at map 0x0033 (1,18) text="Hey, wait u"    dismissed once, never returned
    ... 8+ minutes, no further progress
    killed at 9m44s, never completed

A separate run hit Go's 10-minute default with the test at 9m09s, inside:

    skill.waitForPositionStable                 warp.go:188
    skill.Traverse  Edge{From:0x0d, To:0x32}    warp.go:176
    skill.GoTo -> skill.Travel -> travelFightsThrough

`From:0x0d To:0x32` is Route 2 back into the **south** gate while the leg
targets the **north** gate (0x2F). It oscillates rather than advancing.

Not a timeout bug: raising `-timeout` was tried and only buys a longer stall.
Not a dialogue-retry loop either — the box was logged **once** and dismissed,
so the paging fix works. What is missing is any bound on the composite:

    travelFightsThrough  10 retries
      Travel             maxBattles 10, loops per battle
        GoTo             maxReplans 8
          Traverse       crossBudget + arriveBudget + positionStableBudget(500) frames

## For the next slice

1. Dialogue recovery is necessary but **not sufficient**.
2. The journey needs **one global deadline** so a stuck run reports in seconds.
3. Suspected, unconfirmed: the oscillation may be a regression from S5c-3's
   warp-tile selection. Rule it out before designing around it.

Both conclusions are also in `docs/SLICE6-PLAN.md`.

## Gotchas worth keeping

- `TestGymBoulderBadge` and `TestTravelToPewter` are skipped **on purpose**,
  with pointers. Do not un-skip or delete either; slice 6 starts from them.
- This package outruns Go's 10-minute default. Pass `-timeout` on every
  command that unskips the journey test.
- `gymLeadLevel = 12` is what survives Brock. Do not lower it to fit a budget.
