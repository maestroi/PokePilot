# RUNNOTES — S4-5a LLM planner (done, verified)

## What changed
Commit "agent: an OpenAI-compatible LLM planner that can only pick what was offered".

- `agent/llm.go` (new): `LLMPlanner{BaseURL, Model, Client}`,
  `NewLLMPlanner()` (defaults http://192.168.50.81:8002/v1, model
  qwen3.8-27b; env POKEPILOT_LLM_URL / POKEPILOT_LLM_MODEL override).
- `Next` implements the Planner seam: one POST to
  {BaseURL}/chat/completions, parse choices[0].message.content, then
  `Chosen`. Reply handling: trim, take first integer (prose/fences fine);
  no integer -> error with raw reply; otherwise pass the integer string
  to Chosen, whose error is returned as-is. Never guesses, no retry, no
  fallback to offered[0].
- temperature always 0; nil Client -> 60s timeout.
- Errors name what happened: transport, non-200 (with trimmed body
  snippet), unparseable JSON, empty choices (with snippet).
- System prompt: "You are choosing the next objective for a Pokemon Red
  player. Reply with ONLY the number of your choice. Do not explain."
- User prompt: "Observation:\n" + MarshalIndent JSON + "\n\nOffered
  objectives:\n" + "N: <String()>\n" lines (1-based index, not sentence).
- Stdlib only; go.mod/go.sum byte-identical (verified via git status).

## Verified
- `go build ./...`, `go vet ./...` clean.
- `env -u POKEMON_RED_ROM go test ./agent/` -> ok (ROM-gated tests skip).
- Only agent/llm.go staged/committed; no HANDOFF.md present.

## Gotchas / next
- Work only in this worktree, NOT /home/maestro/Documents/projects/PokePilot
  (that checkout is on main, has no agent/; a prior attempt lost work there).
- No llm_test.go exists yet; the "8/8 tests" from the task brief refer to
  the prior attempt's external tests. A next task may add
  agent/llm_test.go (httptest server, no ROM needed).
- Full-suite runs: still `go test -skip TestGoToViridianPokecenter ./...`
  until plan fdc1544f lands.
- S4-5b likely wires LLMPlanner into Run/cmd with offered lists; LLMPlanner
  is already a Planner (Next signature matches), so no seam changes needed.
