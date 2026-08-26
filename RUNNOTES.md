# RUNNOTES — S4-5c LLM planner error-path + env tests (done, verified)

## What changed
Commit e32daab "agent: LLM planner tests — error paths and env override".

- `agent/llm_test.go`: appended 6 tests below the S4-5b content; helpers and
  existing 2 tests untouched.
  - `TestLLMPlannerOutOfRangeIsError`: reply "7" with 3 offered -> error, and
    result is NOT offered[0] (guards the no-silent-fallback contract).
  - `TestLLMPlannerHallucinationIsError`: reply "go to viridian city" -> error
    that carries the raw reply (no fuzzy match).
  - `TestLLMPlannerHTTPError`: raw httptest server returning 500 -> error
    mentions "500".
  - `TestLLMPlannerBadJSON`: body `{"choices": [` -> error names the JSON
    failure (checked case-insensitively).
  - `TestLLMPlannerEmptyChoices`: body `{"choices":[]}` -> error names
    "choices".
  - `TestNewLLMPlannerEnv`: t.Setenv POKEPILOT_LLM_URL / POKEPILOT_LLM_MODEL
    -> NewLLMPlanner picks them up.

## Verified
- `env -u POKEMON_RED_ROM go build ./...`, `go vet ./agent/` clean.
- `env -u POKEMON_RED_ROM go test ./agent/ -count=1` -> all 8 LLM tests pass,
  no ROM, no inference server (httptest on loopback only).
- Full suite: `env -u POKEMON_RED_ROM go test -skip TestGoToViridianPokecenter
  ./... -count=1` -> ok.
- `git diff go.mod go.sum` empty; only agent/llm_test.go staged/committed.

## Gotchas / next
- Work only in this worktree, NOT /home/maestro/Documents/projects/PokePilot
  (that checkout is on main, has no agent/; a previous attempt lost work there).
- Error-message contracts in llm.go: HTTP error embeds resp.Status; JSON
  error says "reply is not valid JSON"; empty choices says "reply has no
  choices"; no-number says `no number in reply %q`; Chosen says "index %d out
  of range" / "is not one of the offered objectives".
- Next: wire LLMPlanner into Run/cmd with offered lists (S4-5b follow-up);
  LLMPlanner already satisfies the Planner seam (Next signature matches).
- Full-suite runs: still `go test -skip TestGoToViridianPokecenter ./...`
  until plan fdc1544f lands.
