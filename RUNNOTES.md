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

## S7-4: a malformed reply is information, not the end of the run

- `agent/run.go`: new `FeedbackPlanner` interface (`NextFeedback(obs, offered, feedback)`);
  `Run` now routes planning through `planWithRetries`, which re-asks the SAME
  round with the rejection text quoted back, up to `MaxReplyRetries = 3` total
  asks (initial + 2 re-asks). Exhausting them is StopError as before; a plain
  Planner (ScriptedPlanner) that errors keeps the old stop-on-first behavior.
  New `Result.ReplyRetries` counts re-asks across the run, and each rejection
  logs a "round N: reply rejected (ask X of 3)" line.
- `agent/llm.go`: `LLMPlanner.Next` is now `NextFeedback(..., "")`; `ask`
  appends the rejection verbatim to the user prompt ("Your previous reply was
  rejected: ..."). Strictness untouched: planner.go still rejects every
  non-applying argument (gate grep "does not apply" = 5 hits).
- Tests: run_test.go — recovers after one bad reply (StopDone, ReplyRetries=1,
  feedback carries the text) and exhausts at exactly MaxReplyRetries asks
  (StopError, ReplyRetries=2). llm_test.go httptest — second prompt contains
  the rejection text; first does not; `{"choice":1,"level":12}` for a go-to is
  still an error, never coerced. No live model anywhere.
- **Recovery note (this run):** the first attempt's verification FAILED. Two
  bugs, both found by reproducing with POKEMON_RED_ROM set (the fixture Run
  tests DO run when the ROM is available — do not assume they skip):
  1. `planWithRetries` returned StopDone as its stop value and Run checked
     `stop != 0` — but StopDone is `Stop(0)`, so a finished planner read as
     "keep going" and Execute ran on the empty objective (KindGoTo is
     Kind(0), so `Objective{}` renders "go to"). Five fixture tests died with
     StopFailed. Fix: planWithRetries returns the raw error; Run breaks on
     `errors.Is(err, ErrDone)` / `err != nil` directly — never via a stop-value
     check, because StopDone is the zero value.
  2. The test's `replyPlanner.take()` rejected on EVERY ask while rejectErr
     was set, so the "recovers" test never recovered. It now rejects exactly
     `rejects` times (1 for recovery, 10 for exhaustion).
- Verified this time with the ROM: `go build ./...`, `go vet ./...`,
  `grep -q "does not apply" agent/planner.go`, and `go test ./... -count=1
  -short` all green (full suite, no skips in agent/).

### For the next task
- badgerun's table still has no health/retry columns: add them by reading
  `planner.Health` and `res.ReplyRetries` after each run (runResult +
  formatTable in cmd/badgerun). That is the diagnostic that separates a loop
  problem from a capacity problem.
- Any new Stop reason must NOT be appended after StopDone: StopDone is
  Stop(0) and every "is this a stop?" check in Run breaks on the error, not
  on the value.

## S7-5: item and trainer bytes kept by the map parser

- `red/rom/map.go`: `Object` gained `ItemID`, `TrainerClass`, `TrainerSet`.
  The parse that used to `skip(1)`/`skip(2)` now reads those bytes in ROM
  order (trainer: class then roster; item: item id). Nothing else in the
  parser changed. Pure parsing — no emulator, no fixture.
- `red/rom/map_test.go`: two ROM-backed tests (loadROM / POKEMON_RED_ROM):
  Viridian Forest has exactly three 0x80-bit objects at (25,11),(12,29),(1,31),
  all SPRITE_POKE_BALL (0x3D), the (1,31) one ItemID==0x04; Pewter Gym has
  two 0x40-bit trainers at (4,1),(3,6) and exactly one plain NPC — the guide
  at (7,10). Cross-contamination asserted both ways: plain NPCs have
  ItemID/TrainerClass/TrainerSet all 0; items have TrainerClass/Set 0.
- **All ground-truth coordinates and ids matched the real ROM on the first
  run.** No discrepancies to report.
- Verified: `go build ./...`, `go vet ./red/rom/`, `go test ./red/... -count=1`
  green (ROM at /home/maestro/Documents/projects/PokePilot/roms/pokemon_red.gb;
  the worktree has no roms/ dir — point POKEMON_RED_ROM there).

### For the next task
- `ParseMap(rom, mapID).Objects` now carries pickup and trainer payloads, so
  later tasks (item pickup, trainer engagement) read them from the header —
  no re-parsing needed. Item ids are raw ROM constants (0x04 = POKE_BALL);
  `skill.ItemPokeBall` already names that one.
