# RUNNOTES

## S6-12 — Badge harness landed; scoreboard is VOID

### What landed (restored from snapshot commit 3b98489, no new logic)
- `cmd/badgerun/`: sweep harness — llm planner to the Boulder Badge per
  starter/seed, prints a table. Tests cover arg parsing + table format only;
  not part of `go test ./...`.
- `agent/llm.go`: `PromptLog`, `ExtraSystem` (the -inject-fact seam),
  `Timeout` (POKEPILOT_LLM_TIMEOUT). Restored because the harness references
  all three and the current file was exactly the snapshot version minus them.
- `emu/emu.go`: `OnFrame` — headless runs now get the dialogue tape
  (agent.Run's OnSample install was dead code without a Watch).
- `skill/story.go`: GetStarter no longer requires WINNING the rival battle
  (seed-dependent outcome; ROM proceeds identically either way).
- `docs/AGENT.md`: badgerun section. `-inject-fact` defaults OFF, verified in
  code: `fs.Bool("inject-fact", false, ...)`. No `badgerun-out/`, `.log` or
  `.state` file committed.

### The scoreboard is VOID. Four measured reasons:
1. **Single-slot contention.** badgerun and this coding agent share one
   inference slot (server :8002, model qwen3.8-27b). Every 27b sweep died on
   its FIRST call: "context deadline exceeded ... awaiting headers", 0
   frames. GET /v1/models answered in 0.3ms; POST timed out at 90s. Attempt
   1's baseline completed only because it used the default qwen3.5-4b.
2. **No goal in the prompt.** agent/llm.go's system prompt has no objective
   (`grep -ri goal agent/` → nothing). Runs measure an unprompted model with
   a menu.
3. **A superfluous argument kills the run.** Attempt 1's baseline: 9 of 9
   runs ended in error, 27 "argument does not apply" rejections, 0 badges.
4. **No model verification.** chatResponse parses only "choices" — no model,
   no finish_reason — so ablation A cannot detect a different model served.

**NO recall-vs-derivation reading is possible from these runs, and none is
offered.** No completion rate, no capability claim. The real measurement
moves to slice 7 (docs/superpowers/specs/2026-08-29-slice7-design.md) after
the goal, reply validation and rejection recovery land.

### For the next task
- Smoke run NOT executed: 192.168.50.204:8000 returns 401 without a bearer
  token and no `.env`/`llm_token` exists in this worktree. Harness verified
  via build + vet + `-short` suite (all ok, 0 skips).
- Do NOT run sweeps from inside agent-runner (single-slot contention).
- S6-11 (prev): per-objective checkpoint ring in agent.Run; plain
  `fixture.LoadState`.

## S7-1: fixture cache survives a clean worktree

`fixture.Dir` replaced by `DefaultDir` + `ResolveDir()` in
skill/fixture/fixture.go; `POKEPILOT_FIXTURE_DIR` overrides the cache
location. `FailureDir` untouched. Two unit tests added to fixture_test.go.

Cache-reuse measurement (same POKEPILOT_FIXTURE_DIR=/tmp/pokepilot-fixtures,
POKEMON_RED_ROM set, `go test ./skill/fixture -count=1`):

- cold (empty dir):  **43.227s**
- warm (reused):     **1.708s**

/tmp/pokepilot-fixtures now holds: reds_bedroom.v5.state,
post_starter.v5.state, pallet_town.v5.state, viridian_city.v5.state,
viridian_pokecenter.v5.state.

For the next task: set POKEPILOT_FIXTURE_DIR to a shared path (e.g.
/tmp/pokepilot-fixtures) when running the suite — clean worktrees no longer
rebuild post_starter/post_pokeballs from boot.

## S7-2: Tell the planner what it is trying to do

### What landed
- `agent/llm.go`: `LLMPlanner.Goal` (task statement, explicitly NOT strategy).
  New `systemPrompt()` method: with a Goal the prompt is
  `Your goal: <goal>\n\n` + the old prompt; without one it is byte-identical
  to `llmSystemPrompt` (+ ExtraSystem) — asserted in a test, not prose.
- Exact goal sentence shipped: **`Earn the Boulder Badge.`** The solution is
  STILL WITHHELD: no starter, no Pokemon, no type hint anywhere in the prompt
  path (gate grep on agent/llm.go clean).
- `cmd/badgerun`: `-goal` FLAG (not a constant), defaulting to
  "Earn the Boulder Badge."; set on the planner in plannerFor; echoed in the
  run banner. `-inject-fact` stays OFF by default and separate from the goal.
- Tests: agent/llm_goal_test.go (internal pkg, byte-identity + goal rendering)
  and two agent_test cases via httptest (goal in system prompt; goal does not
  alter reply parsing). No live model anywhere.

### For the next task
- Every prior badgerun row was scored WITHOUT a goal; new rows carry one.
  They are not comparable — say so when reading them side by side.
- Empty-Goal behavior is byte-identical, so `-goal ""` reproduces the old
  prompt exactly if an ablation needs it.

## S7-3: Check that the model answering is the model we asked for

### What landed (agent/llm.go, agent/llm_test.go; httptest only, no live model)
- `chatResponse` now parses `model`; `chatChoice` parses `finish_reason`.
  `ask` returns a `chatResult{Content, Model, FinishReason}`.
- **Model mismatch is a hard typed error** (`ErrModelMismatch`) naming BOTH
  requested and answering models — never coerce, never warn-and-continue.
  This is what makes ablation A's "not capacity" reading trustworthy.
- **Omitted `model` field is accepted** (some servers omit it) but logged
  ONCE per run: "cannot verify which model answered".
- **`finish_reason` present and != "stop" (e.g. "length") is a typed
  rejection** (`ErrNotFinished`), not parsed. Truncated-JSON-still-parses
  silent wrong answers are gone.
- **Fallback tightened** (`looksLikeAnswer`): non-JSON reply accepted only
  if, after think-block stripping + trim, <= 12 chars, exactly one integer,
  rest whitespace/punctuation. "2" and "  3 " pass; "rate limited, retry in
  5 seconds" is REJECTED (was choice 5), as is any prose with a digit.
  Consequence: old tests expecting prose acceptance now expect rejection —
  the scratch-work test keeps only the closed-think-block + bare-number case.
- **`LLMPlanner.Health LLMHealth{Transport, Rejected, Fallbacks}`**: per-run
  counters (transport/timeout/non-200/bad envelope; shape rejections incl.
  mismatch + finish_reason + unparseable; fallback-path uses). Exported on
  the planner; badgerun should print them on each scoreboard row in S7-4
  (not done here — task scope was agent/ only).

### Tests (all httptest)
mismatch rejected naming both models; missing model accepted + once-only log;
finish_reason length/content_filter rejected, stop + omitted accepted;
prose-with-digit rejected (3 cases); bare "2" fallback still works;
Health bucket counts. Full `-short` suite green.

### For the next task (S7-4: make rejections retryable)
- `errors.Is(err, agent.ErrModelMismatch)` / `agent.ErrNotFinished` are the
  hooks; both wrap with detail. Rejected replies are already counted in
  `planner.Health.Rejected`.
- badgerun's table has no health columns yet — add them there (runResult +
  formatTable), reading `planner.Health` after each run.
