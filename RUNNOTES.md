# RUNNOTES — S4-4 Run loop (done, verified)

## What changed
Commit "agent: the bounded observe-plan-execute loop".

- `agent/run.go` (new): `Stop` (StopDone/StopStuck/StopBudget/StopError),
  `Result{Stop, Rounds, Completed, Err, Final}`, `Budget{MaxRounds,
  MaxFrames, StuckAfter, Log}`, and `Run(m, romData, p, offered, budget)`.
- Loop: observe -> p.Next -> Execute -> observe, per round.
  - ErrDone -> StopDone (success). Planner's other errors -> StopError.
  - Objective error -> StopError, Err kept, no retry, no next objective.
  - Stuck: map/x/y/party count/event list compared before/after each
    objective; StuckAfter consecutive unchanged (default 3, field on
    Budget, not hardcoded) -> StopStuck.
  - Zero MaxRounds or MaxFrames -> StopError with an error, not unlimited.
  - Frame budget checked after each round via m.FrameCount() delta.
  - Completed appended only after a successful objective.
  - One log line per round when Budget.Log != nil:
    `round N: <objective String()> -> map %02x at (x,y)`.
- `agent/run_test.go` (new, package agent_test, ROM-gated via the existing
  loadFixture): TestRunDone, TestRunRoundBudget, TestRunError, TestRunStuck.

## Verified
- No ROM: `env -u POKEMON_RED_ROM go test ./agent/ -run TestRun` -> all 4 SKIP.
- With ROM (POKEMON_RED_ROM=/home/maestro/Documents/projects/PokePilot/roms/pokemon_red.gb):
  all 4 PASS. TestRunDone log lines, verbatim:
    round 1: take a starter -> map 28 at (5,6)
    round 2: go to pallet town -> map 00 at (5,6)
  (map 28 = Oak's lab after the starter; 00 = Pallet Town.)
- `go test -skip TestGoToViridianPokecenter ./...` -> all packages ok.

## Gotchas / next
- TestRunStuck asserts Rounds == 4: round 1 walks bedroom -> Pallet Town
  (a change), then 3 unchanged repeats trip default StuckAfter (3). If the
  fixture start position or default changes, update that assertion.
- Stuck test works directly from the reds_bedroom fixture: the walk to
  Pallet Town needs no starter first (bedroom -> house -> town is connected).
- S4-5 (model planner): wire a real Planner into Run with offered lists and
  a Log writer; handle ErrDone explicitly. Keep Chosen's exact-match rule.
- Full-suite runs: still `go test -skip TestGoToViridianPokecenter ./...`
  until plan fdc1544f lands. Work only in this worktree.
