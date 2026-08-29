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

## S7-6: skill.Pickup — collect an item ball and prove the bag grew

### What landed
- `skill/pickup.go`: `Pickup(m, romData, x, y, want)` — reads the bag count
  for `want` BEFORE (state.DecodeInventory), walks to a walkable adjacent
  tile via GoTo (`approachItem`; no-op when already adjacent, so there is no
  second approach path), Face, press A, page the box closed.
  - Postcondition is POSITIVE: the count must be exactly before+1, else
    `ErrBagNotRisen` naming both counts. An already-collected ball fails
    here, cleanly — deliberately unfiltered (no item event flags exist;
    no decoder added, per spec).
  - Yes/no trap: every page pass checks `state.DecodeTwoOptionMenu` BEFORE
    the next A and returns `ErrPickupMenu` naming the cursor option — never
    presses A into a menu (the S6-3/S6-4 lesson).
- `agent/objective.go`: `KindPickup` (X, Y, Item) appended after KindBuy so
  no existing kind renumbers; Execute calls skill.Pickup; Validate checks
  ItemName; String renders "pick up the POKE BALL at (x,y)".
- `skill/pickup_test.go`: TestPickupPokeBall — post_errand fixture (Viridian
  City, the same start TestGymBoulderBadge uses for this crossing), Travel to
  the forest, Travel to (2,31) east of the ball at (1,31) on 0x33, Pickup,
  assert POKE BALL count rose by one. `testing.Short()`-skipped; proven
  separately: `-run '^TestPickupPokeBall$'` PASS (2.2s).

### Verified
`go build ./...`, `go vet ./skill/ ./agent/`, both gate greps in
skill/pickup.go (DecodeInventory, DecodeTwoOptionMenu), `go test ./skill/...
./agent/... -short -count=1` all green.

### For the next task
- The city -> Route 2 crossing is STOCHASTIC: Viridian City has two walkers
  (SAILOR, DAISY) patrolling the plaza north of (19,8). One run from
  post_starter died there: "You can't go through here!" re-opened on every
  retry, Travel gave up after 10 dialogue recoveries. The same crossing from
  post_errand passed. If a journey test flakes at (19,9) on map 0x01 facing
  up, that is the plaza, not your code — dump says so via PROBE_STATE.
- `approachItem` picks the first walkable orthogonal neighbor in
  Up/Down/Left/Right order; for the (1,31) ball that is (2,31), the tile the
  test stands on. If a future ball has only one reachable neighbor behind a
  grass band, Travel (not GoTo) belongs in the approach instead — GoTo
  aborts on wild battles by design.

## S7-7: planner is offered the people and items on its map

### What changed
- `agent/observe.go`: `Observation.MapObjects []MapObject` (X, Y, Kind
  "person"|"item"|"trainer", Item name). New `agent.MapObjects(romData,
  mapID)` reads the ROM map header via `rom.ParseMap` — NOT sprite RAM.
  Classification from the TextID bits: 0x80 item (named via the bag table,
  unknown id says "item %d"), 0x40 trainer, else person.
- `agent/offer.go`: Offer appends `KindTalk{X,Y}` per person and
  `KindPickup{X,Y,Item}` per item. TRAINERS ARE REPORTED BUT NOT OFFERED
  (no fight verb exists; offering one is a guaranteed failed objective).
  Collected items are NOT filtered — no data source; a vanished ball fails
  Pickup's bag postcondition as an ordinary failure.

### Object counts, Pallet -> Pewter route (measured, real ROM)
pallet_town 0x01: 7 (7 person) | route_1 0x02: 5 (5 person) |
viridian_city 0x03: 11 (10 person, 1 trainer) | route_2 0x2f: 2 (2
person) | pewter_city 0x30: 2 (2 person). **Largest is 11 — small. No cap
added.**

### Verified
`go build ./...`, `go vet ./agent`, `go test ./agent -count=1` green
(55s). Gate: zero occurrences of the sprite-RAM decoder identifier in
agent/observe.go and agent/offer.go. New tests: TestMapObjectsFromROM
(Viridian Forest 0x33 = 3 items incl. pokeball at (1,31); Pewter Gym 0x36 =
person at (7,10) + 2 trainers), TestOfferMapObjects (talk at (7,10) and
named pickup offered; no trainer objective).

### For the next task
- The fightable-trainer verb (S7-8?) must add a Kind and an Offer branch;
  MapObjects already reports trainers with coordinates.
- `skill.Pickup`'s bag postcondition is the only guard against collected
  items being re-offered; that stays by design.

## S7-8: TestGymJourneyAffordances — Pallet -> Brock with both affordances used

### What landed
- `skill/journey_test.go`: TestGymJourneyAffordances. One run, post_errand ->
  forest ball pickup (bag POKE BALL 0->1 via state.DecodeInventory) -> train
  lead to L12 -> Pewter Center heal -> gym guide talk at (7,11)/(7,10) ->
  exit, heal, re-enter -> Brock. -short-skipped; proven by its own -run gate:
  PASS in 58s (run 7).

### Cool Trainer re-arms on every crossing — route around him
PEWTERGYM_COOLTRAINER_M sits at (3,6) facing right (sight line x=4..8,
engage distance 5), and his defeat flag is set ONLY by Brock's victory
script (PewterGym.asm:79 .gymVictory) — his own end-battle text sets no
event. Every crossing of row 6 on his sight line is a fresh two-Pokemon
fight (Diglett L11 + Sandshrew L11); a one-Pokemon party cannot pay that
tax in, out for the heal, and back in. The test instead routes through the
x=1 side corridor: entrance -> (1,8) -> (1,4) -> (4,2), and the guide
approach returns (4,2) -> (1,4) -> (1,8) -> (7,11). Every waypoint leg is a
Manhattan-distance shortest path whose bounding box never includes row 6
x=4..8 (all eight legs probed on map 0x36), so no battle can start on them
regardless of BFS tie-breaks. Run 7: every gym leg arrived with battles=0
and the lead faced Brock at full HP deterministically.

### emu.StepFrames(n) never fires the onFrame hook — Talk-driven talks are invisible
The reason the guide's line was missing from the tape in runs 1-6.
`Emu.StepFrames` (emu/emu.go) batch-steps via `m.e.StepFrames(n)` and does
NOT call onFrame; only single-frame `StepFrame` does. skill.Talk pages its
whole conversation with `StepFrames(talkSettle)`, so a conversation Talk
drives is invisible to every per-frame sampler — agent.Run's dialogue tape
included. The journey test therefore drives its own talk: one StepFrame per
loop iteration, Talk's own tap/40-frame-settle cadence, A-taps only while
wFontLoaded != 0, done after 30 controllable frames. (A StepFrames that
stepped through the hook — or a Talk with an onFrame-visible pacing — would
fix this at the source; out of scope here.)

### wFontLoaded IS set during PrintText dialogue on this ROM
Measured: 1 from within one frame of the opening A press until the final
close, so DecodeDialogue and DecodeTwoOptionMenu both see the guide's boxes
(the yes/no menu decoded live, cursor on option 0; the next A tap confirms
the default YES, which is the branch the script takes). Caveat: the vendored
decompilation shows only DisplayTextIDInit setting BIT_FONT_LOADED
(display_text_id_init.asm:34) — a decomp/ROM discrepancy worth resolving,
but empirically the gate passes. Note for future trace work: the earlier
"PrintText never sets wFontLoaded" reading was itself an artifact of the
StepFrames blindness above — the trace had no samples inside the talk.

### Text facts
- The `#` control code ($54) expands to "POKé" at runtime (charmap $BA =
  é), so "#MON champ!" decodes as "POKéMON champ!". Assertions match the
  ASCII on either side of the glyph ("MON champ", "MON LIST").
- The tape records every typing prefix as a settled line once two samples
  agree (224 lines for this one conversation) — the same prefix-capture
  semantics agent.Run's tape has; substring matching is the right assertion.

### Brock fight: lost, and that is the game answering
Run 7: lead L12, HP 36/36, no status, outcome=ResultLost, no badge. Both
affordances were proved from RAM before the fight, so the test logs a
one-line FINDING and passes (AGENTS.md: a game outcome is not a defect; the
rDIV-seeded RNG makes this cycle-chain-dependent).

### Verified
`go build ./...`, `go vet ./skill`, `go test ./skill -short -count=1` green,
`-run '^TestGymJourneyAffordances$'` PASS (58s), `-short` skips it. No
zz_*_test.go left behind; no ROM/.gb/.sav/.state committed.

### For the next task
- If the agent's tape must capture NPC PrintText dialogue, fix the sampling
  seam: make StepFrames step through onFrame (or give Talk an observable
  pacing). Until then, anything asserted from a per-frame sampler about a
  Talk-driven conversation is asserting on frames that were never sampled.
- The decomp/ROM wFontLoaded discrepancy (only DisplayTextIDInit sets it in
  the vendored source; the ROM sets it for PrintText boxes) should get its
  own note before anyone "simplifies" DecodeDialogue's gate off it.
