# RUNNOTES — S4-6 wire agent loop into cmd/pokepilot (done, verified)

## What changed
Commit ab70d3d "cmd: run the objective loop behind -planner" (4 files).

- `cmd/pokepilot/main.go`: new `-planner` flag (default `scripted`).
  - `scripted`: original flow verbatim, moved into `runScripted`
    (boot -> GetStarter -> one GoTo -> hold serving); `report` reused.
  - `llm`: `runLLM` offers KindStarter + one KindGoTo per
    `skill.PlaceNames()`, runs `agent.Run` with `agent.NewLLMPlanner()`
    and Budget{MaxRounds: 32, MaxFrames: 8h@60fps, Log: os.Stdout},
    prints stop reason / rounds / completed / error; StopError or
    StopStuck -> os.Exit(1). `stopName` helper (agent.Stop has no
    String()).
- `skill/goto.go`: hoisted function-local `places` map to package var;
  added exported `PlaceNames() []string` (sorted) — skill had no way to
  enumerate place names, so the list is not duplicated in cmd.
- `Makefile`: `run-llm` target (require-rom, `-planner llm -fps 60`).
- `docs/AGENT.md`: new (repo has no README.md; task allowed this).

## Verified
- `go build ./...`, `go vet ./...` clean; my files gofmt-clean (4
  pre-existing files are not — left alone).
- `env -u POKEMON_RED_ROM go test -skip TestGoToViridianPokecenter
  ./... -count=1` -> all ok.
- Scripted (ROM from /home/maestro/Documents/projects/PokePilot/roms/
  pokemon_red.gb): `make run ARGS='-goto "pallet town"'` -> full flow,
  prints "arrived.", exit 0. No model called in this path.
- LLM, unreachable: `POKEPILOT_LLM_URL=http://127.0.0.1:9 make run-llm`
  -> "planner: llm — the model picks from 6 offered objectives", then
  "run stopped: error after 0 round(s)" + "error: agent: llm planner:
  POST http://127.0.0.1:9/chat/completions: ... connection refused";
  binary exits 1 (make: Error 1). No real inference server called.

## Gotchas / next
- Work only in this worktree, NOT /home/maestro/Documents/projects/PokePilot.
- llm-mode starter is always Squirtle (agent.Execute hardcodes it);
  -starter is scripted-only (documented in docs/AGENT.md).
- llm run ends on budget (LLMPlanner never returns ErrDone) unless it
  errors/sticks; StopBudget exits 0.
- Route 1 still broken (plan fdc1544f): full-suite runs need
  `-skip TestGoToViridianPokecenter`.
