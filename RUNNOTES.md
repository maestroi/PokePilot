# RUNNOTES — S4-5b LLM planner tests (done, verified)

## What changed
Commit 938e829 "agent: LLM planner tests — happy path and prose stripping (httptest, no ROM)".

- `agent/llm_test.go` (new, package agent_test): shared helpers + 2 tests.
  - `llmObs()` fixture: Map 0x28, OAKS_LAB, (5,6) down, Party 1 (Species 1,
    Lv 5, 20/20 HP), Money 3000, Events ["got a starter"].
  - `llmOffered()`: {KindGoTo "pallet town"}, {KindStarter}, {KindTalk (3,1)}.
    Deliberately NOT starting with bare KindStarter (zero Objective ==
    {Kind: KindStarter}; out-of-range test must distinguish "none" from offered[0]).
  - `startModelServer(t, reply, *capture)`: httptest OpenAI endpoint; fails on
    wrong path (/chat/completions) or Content-Type (application/json); captures
    raw request body when non-nil. `llmPlanner(srv)` -> LLMPlanner{BaseURL: srv.URL, Model: "qwen3.8-27b"}.
  - `TestLLMPlannerPicksOfferedObjective`: reply "2" -> offered[1]; asserts body
    contains model/temperature/roles + "1: go to pallet town", "2: take a starter",
    "3: talk at (3,1)".
  - `TestLLMPlannerStripsProse`: reply "I choose 2 because it is closer." -> offered[1].
- llm.go untouched.

## Verified
- `env -u POKEMON_RED_ROM go build ./...`, `go vet ./agent/` clean.
- `env -u POKEMON_RED_ROM go test ./agent/ -count=1` -> ok (all tests, no ROM,
  no inference server; everything goes to httptest on loopback).
- Exact request body captured via a temp dump test (deleted after; not committed);
  matches the contract body in the S4-5b brief byte-for-byte.

## Gotchas / next
- Work only in this worktree, NOT /home/maestro/Documents/projects/PokePilot
  (that checkout is on main, has no agent/).
- Next task (S4-5c?) adds the remaining llm tests (out-of-range, no-number,
  non-200, empty choices, wrong path/Content-Type, etc.) to the SAME file —
  reuse llmObs/llmOffered/startModelServer/llmPlanner as-is.
- Full-suite runs: still `go test -skip TestGoToViridianPokecenter ./...`
  until plan fdc1544f lands.
- S4-5b follow-up: wire LLMPlanner into Run/cmd with offered lists; LLMPlanner
  already satisfies the Planner seam (Next signature matches).
