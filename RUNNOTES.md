# RUNNOTES — S4-3b Planner tests (done, verified)

## What changed
Commit 74a0ace "agent: planner tests, pure logic, no ROM needed".

- `agent/planner_test.go` (new, exact content per task spec), package
  `agent_test` (external test package). No ROM, no fixture, no emulator,
  no `loadFixture` reuse.
- `TestScriptedPlannerOrder`: feeds deliberately wrong obs/offered into
  `NewScriptedPlanner(...).Next`, asserts objectives come back in order,
  then ErrDone, and that ErrDone is sticky (list does not reset).
- `TestChosenMatches`: exact String(), different case, trimmed input, and
  bare 1-based indices "1"/"2"/"3".
- `TestChosenRejects`: empty, whitespace-only, unknown name, index 0,
  out-of-range index, one-char near miss — every case must error AND the
  error must contain the offered objective's sentence (S4-5 safety
  property: errors name the offered list).

## Verified (worktree, NOT the stale PokePilot checkout)
- `env -u POKEMON_RED_ROM go test -v ./agent/ -run 'TestScriptedPlannerOrder|TestChosenMatches|TestChosenRejects'`
  → all three `--- PASS`, `ok github.com/maestroi/pokepilot/agent`.
- ROM set (`POKEMON_RED_ROM=/home/maestro/Documents/projects/PokePilot/roms/pokemon_red.gb`,
  file referenced only, no commands run there):
  `go test -skip TestGoToViridianPokecenter ./...` → all packages ok.
  TestGoToViridianPokecenter is the known red (plan fdc1544f); it has NO
  built-in skip, so "skipped deliberately" = pass `-skip` to `go test`.

## Gotchas / next
- Work ONLY in this worktree. /home/maestro/Documents/projects/PokePilot
  is on an older commit without agent/ — commands there fail.
- The ROM lives at /home/maestro/Documents/projects/PokePilot/roms/pokemon_red.gb;
  the worktree has no roms/ dir. Point POKEMON_RED_ROM at that file for ROM runs.
- S4-3c+/S4-5: anything consuming Planner must handle ErrDone explicitly;
  Chosen stays exact-match + bare index, offeredList in every error.
- Full-suite runs: use `go test -skip TestGoToViridianPokecenter ./...`
  until plan fdc1544f lands.
