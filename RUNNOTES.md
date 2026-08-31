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
moves to slice 7 (docs/plans/2026-08-29-slice7-design.md) after
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

## S7-9: correcting false facts slice 6 committed (doc-only)

### The "pokecenters have no PC" claim is FALSE
S6-4's RUNNOTES justified abandoning its acceptance test on the grounds
that "this decompilation's pokecenters have no PC machine sprite (no
SPRITE_PC constant, no PC in any Pokecenter object file)". Wrong: the PC
is not an NPC sprite at all — it is a TILE-ACTIVATED HIDDEN EVENT.
`~/.cache/pokered/data/events/hidden_events.asm` has, for VIRIDIAN, PEWTER
and CERULEAN (and the other Centers): `hidden_event 13, 3,
OpenPokemonCenterPC, SPRITE_FACING_UP`. The PC sits at tile (13,3) of every
Center and is used by standing on it facing UP; depositing a mon IS
possible in-rom. The search missed it because it looked for a sprite in the
object files — searching object files for a sprite is not evidence about
hidden events. The fact now lives in docs/AGENT.md under "## ROM facts".

### RUNNOTES has been getting wiped
Five slice-6 tasks in a row REPLACED this file instead of appending, so
each task's measurements vanished from the tip. S6-6 nearly shipped a guess
at balls-per-catch because its instructions said "read S6-3's RUNNOTES
numbers" and they were gone. The deleted sections are recoverable in git:

    S6-0f  82880c7:RUNNOTES.md
    S6-3   f5300305:RUNNOTES.md   (the balls-per-catch table S6-6 needed)
    S6-4   8a64e468:RUNNOTES.md

Do NOT reconstruct them from memory — a summary written from recollection
is a new source of wrong facts, which is the exact problem this task fixes.

Rule, stated plainly: **APPEND to this file, do not replace it.**

### For the next task
- No Go changes were made; verification is `git diff --quiet HEAD -- "*.go"`.
- Do not design around "there is no PC in Centers"; the PC exists as a
  hidden event at (13,3), faced UP.

## S8-1: StepFrames now runs per-frame hooks (emu/emu.go)

`Emu.StepFrames` previously batched (`m.e.StepFrames(n)`) and called NEITHER
`onFrame` nor `onSample`, so every conversation `skill.Talk` drives via
`StepFrames(talkSettle)` was invisible to per-frame samplers — including
agent.Run's dialogue tape. Now, when a hook is installed, it loops
`StepFrame()` n times (which already does capture + throttle per frame).
With no hook it keeps the fast batch path unchanged.

The StepFrames hook condition matches StepFrame's exactly:
`onFrame != nil || (trace == nil && onSample != nil)`.

### Suite timing, `go test ./skill -short -count=1`
- Before: real 0m0.595s (go test reports 0.005s)
- After:  real 0m0.605s (go test reports 0.005s)

Within noise — the no-hook fast path is untouched and nothing unexpected
installs a hook in the -short suite.

### New tests (emu/emu_test.go)
- `TestStepFramesCallsOnFrameEveryFrame` — OnFrame installed, StepFrames(10),
  hook ran exactly 10 times. Verified it FAILS on pre-change code (0 calls).
- `TestStepFramesNoHookAdvancesInBatch` — no hook, FrameCount advances by 10.
- `TestStepFramesCallsOnSampleEveryFrame` — OnSample + no trace, called n times.

### For the next task
- A Talk-driven conversation is now visible to a per-frame sampler: whatever
  S7-7's planner offered (KindTalk at the gym guide) can now land in
  `Observation.RecentDialogue`. skill.Talk itself was intentionally not
  touched — the seam is the emulator's.

## S8-2: Battle answers the "forget a move?" prompt on purpose

### What landed
- `skill/battle.go`: `useNextMonUp` generalized to `twoOptionPromptUp`
  (marker "Use next" OR "trying to learn") — ONE detection path for both
  yes/no prompts, per the task; answering still waits on
  `state.DecodeTwoOptionMenu` seeing the drawn cursor. New `forgetMenuUp`
  case ("forgotten?") drives the move list after a YES: `selectForgetSlot`
  steps the cursor and verifies `wCurrentMenuItem` after every tap (A only
  once the cursor reads the target — it cannot use SelectMenuItem, which
  treats wMaxMenuItem as exclusive while this menu stores
  wNumMovesMinusOne, 3 for four moves). Policy is `forgetSlot`: replace the
  LOWEST slot that is not the mon's only damaging option (power > 0 via
  `rom.LookupMove`; an unknown id counts as damaging, the same safe reading
  StatAwareMove uses); a pick the ROM bounces as an HM technique marks that
  move tried and retries the next slot; all-bounced fails loudly.
- `skill/battle_test.go`: TestBattleAnswersForgetMovePrompt — positive,
  RAM-read assertion: after the grind the move set is exactly
  `[BITE TAIL_WHIP BUBBLE WATER_GUN]` (slot computed by the stated policy,
  not hardcoded). -short-skipped; proven by its own -run gate.
- Stale comment fixed at `skill/train.go:57-59` (the prompt is answered on
  purpose now, and the level-22 claim corrected — see below).

### Two measurements that changed the task's own premise
1. **The offer comes at level 24, not 22.** The fixture's level-15 SQUIRTLE
   evolves into WARTORTLE at 16, and LearnMoveFromLevelUp reads the CURRENT
   species' learnset (`wPokedexNum = wCurSpecies`): SquirtleEvosMoves says
   `db 22, BITE`, WartortleEvosMoves says `db 24, BITE`. Measured: a grind to
   22 shows "grew to level 22!" + the stats box and then the battle ends
   with NO prompt at all. So S6-4's RUNNOTES diagnosis — "the prompt was
   shown and dismissed by the A-tap" — was itself a stale note: no prompt
   ever fired on that line's path to 22. The test targets 24, and the false
   level-22 claims in train_test.go / train.go were corrected.
2. **The prompt text has a `<CONT>` wait.** TryingToLearnText is
   `"<NAME> is" / "trying to learn" <CONT> "<MOVE>!"` — the first half
   BLOCKS on a button press before "BITE!" and the yes/no box are drawn. A
   bare StepFrame in the detection case spins (measured: 60,000-frame cap
   stuck on the half-finished line, `max=5` stale from the move menu). The
   case now taps A while the cursor is not yet drawn; for "Use next #MON?"
   (no wait) the tap is harmless, and if it lands after the box is drawn it
   answers YES at cursor 0 — the same answer SelectMenuItem gives.

### Result (measured, ZBAT=1, real ROM)
- `moves [33 39 145 55] -> [44 39 145 55]` — TACKLE/TAIL_WHIP/BUBBLE/
  WATER_GUN become BITE/TAIL_WHIP/BUBBLE/WATER_GUN; BITE (0x2c) in slot 0,
  the lowest slot (three of the four moves deal damage, so no slot is
  protected). 343 battles across 5 blackout-split segments; PASS in 263s.
- The tape shows the whole episode: "is trying to learn" -> YES ->
  "Which move should be forgotten?" (cursor 0) -> "1, 2 and... Poof!" ->
  "learned BITE!" with `moves=[{44 25} ...]` in RAM.

### Verified
`go build ./...`, `go vet ./skill/`, `go test ./... -count=1` all green
(POKEMON_RED_ROM at the sibling checkout). NOTE: the skill package now takes
~15 min end-to-end and FAILS under go test's default 10-minute per-package
timeout — run it with `-timeout 40m` (measured: ok in 919s).

### For the next task
- The forget-menu cursor is `wCurrentMenuItem` over an INCLUSIVE
  wMaxMenuItem (= numMoves-1); any future helper that reuses SelectMenuItem
  for it will reject the last slot. selectForgetSlot exists for that reason.
- Level-up move offers on a line are a property of the CURRENT species'
  table, not the starter's: check EvosMoves for the evolved form before
  predicting which level a prompt fires at.

## S8-3: Route 1 → Pallet Town edge crossing (2026-08-29)

Swarm failure, three identical runs (seed 0, frame 17571):
`connection edge 0c->00 via south did not cross within 180 frames; still on
map 0c at (10,35)`. Every run ended later at Oak's lab (5,6) — the player
recovered after the error, which pointed at a battle that was active but
never reported as one.

**The destination-walkability hypothesis is REFUTED.** The four probe
answers, measured with `TestProbe` and a walkability sweep over both seam
rows (Route 1 row 35, Pallet Town row 0):

- Route 1 (0x0c) row 35 walkable columns: 0,1,2,4,5,6,7,8,10,11,13,14,15,
  16,17,19; unwalkable: 3,9,12,18.
- Pallet Town (0x00) row 0 walkable columns: 0,1,2,4,5,6,7,8,10,11,13,14,
  15,16,17,19; unwalkable: 3,9,12,18. Identical to the source row, as a
  connection pair should be.
- x=10 is walkable on BOTH sides, and the crossing works: from (10,35)
  facing down, holding down crosses in 17 frames (Pallet→Route 1→Pallet
  round trip passes).
- `edgeTarget` from `Place("route 1")`=(5,14) picks exactly (10,35)
  (`PROBE_MAP=0x0c PROBE_AT=5,14`: south edge nearest reachable tile
  (10,35)) — the swarm's tile was never a bad choice.

**What actually happens:** Route 1's south edge is TALL GRASS at x=10 and
x=11 — stepping onto (10,35) or (11,35) fires wild encounters (measured:
12/40 and 3/12 rDIV phases respectively), and Pallet Town's row 0 has the
same grass at the same columns ((10,0): 6/12, (11,0): 2/12). The walk to
the edge therefore ends on a step ONTO an encounter tile, and that step is
WalkPath's LAST one: its `DecodeBattle` check can land a few frames before
the encounter fires. A phase sweep (40 fixture replays with shifted rDIV
phases — a fixture replay is bit-identical, so without the shift every run
rolls the same game) found 12/40 encounters, of which 4 fired at push frame
1, after the walk had returned: the player frozen at (10,35) on map 0x0c
while the push held its button for 180 frames. That is the swarm error
verbatim, and it explains the "multiple consecutive" pattern: same seed →
same rDIV phase → same encounter.

**The fix** (skill/warp.go): the push phase now watches `wInBattle` as well
as the map flip — shared by both warp and connection edges, so no parallel
path. When a battle is active it releases the button and returns the
normalized `ErrBattle`, exactly like a battle on the walk: Travel fights it
and re-plans from the same tile, where no second encounter can fire because
the player is already standing on the grass. The timeout message is
unchanged.

**Verification:** the same 60-phase sweep through the real `skill.Travel`
path fails 8/60 on the old push with the swarm's error string verbatim, and
passes 60/60 with the fix (16 iterations fought a battle and recovered).
`TestTravelRoute1ToPallet` (skipped under -short) pins the edge: Route 1
→ Pallet Town, asserting arrival positively — wCurMap == 0x00 and the
player controllable.

## S8-4: does Travel survive a trainer ambush mid-route? (Route 3, 0x0E)

Measurement task — no fixes, no new Kinds, no sight-line avoidance. The
question is what `skill.Travel` does when a stationary trainer's line of sight
engages the player while it is walking Route 3, and whether the defeated
trainers re-arm on a return crossing. (Slice 8 candidate item 1,
docs/SLICE8-CANDIDATES.md.)

### Setup constraint (why the measurement ran the whole Brock journey first)
Route 3 is reachable only from Pewter City's east edge, and
`PewterCityDefaultScript` (`scripts/PewterCity.asm`) re-fires a forced
"Go take on Brock!" text box **every frame** while the player stands on any of
(35,17)/(36,17)/(37,18)/(37,19) until `EVENT_BEAT_BROCK` is set. So no
pre-Brock fixture can reach Route 3 at all — the first measurement attempt
stalled on that dialogue in Pewter. The setup therefore ran the
`TestGymBoulderBadge` journey (train the lead to L12, gym, badge) once and
cached a post-Brock checkpoint; the two crossing legs then ran from it.
`EVENT_BEAT_BROCK` is event flag 0x77 = byte 14, **bit 6** of `wEventFlags`
(derived from `event_constants.asm`, not counted).

### Route 3 geometry (measured with TestProbe on the live ROM)
A **70×18** east-west corridor. West edge → Pewter City (0x02); north edge
(59,0) → Route 4 (0x0f). No tall grass anywhere — every battle on the map is a
trainer. The player enters from the west edge (near (0,9)); the far side used
as the destination was (59,1), past every trainer. The object file has **eight
OPP_ trainers** at x=10..33 plus one plain NPC (Super Nerd at (57,11)):

    (10,6) YOUNGSTER RIGHT  L4   (14,4) YOUNGSTER DOWN   L1
    (16,9) COOLTRAINER LEFT L1   (19,5) YOUNGSTER DOWN   L5
    (23,4) COOLTRAINER LEFT L2   (22,9) YOUNGSTER LEFT   L2
    (24,6) YOUNGSTER RIGHT  L6   (33,10) COOLTRAINER UP  L3

### The five answers
**1. Does the battle start at all when a trainer's line of sight engages
mid-route — or does Travel report ErrBlocked / a step timeout?**
The battle starts normally; there is no ErrBlocked and no step timeout.
The exact sequence WalkPath sees: the in-progress step completes (its
coordinate-change predicate is satisfied even though the trainer walk-up sets
`wJoyIgnore`), then WalkPath's post-step check finds the trainer's pre-battle
text box and returns `ErrDialogueInterrupted`. Travel recovers that dialogue
(`RecoverDialogue` pages it), and the next `GoTo` sees `wIsInBattle` and
returns `ErrBattle`. So the first signal is the pre-battle text box, not a
block; the battle then runs.

**2. Does `Battle(m, policy)` drive the trainer battles to a result — win or
lose?**
Yes. All six were driven to `ResultWon`. No losses in the measured run (the
lead was ~L16 entering Route 3 and every trainer is L1–L6, so it won cleanly).

**3. After each battle, does Travel resume the route and reach the
destination?**
Yes. Each victory leaves the player on the encounter tile; Travel re-plans
from there and continues. Leg 1's `Replans` list showed exactly six replans at
the six encounter tiles, and the final position was exactly the destination
(59,1), controllable.

**4. Once defeated, do the trainers re-engage on a return crossing? Are they
flagged / hidden?**
No re-engagement. The return westward crossing (leg 2) fought **0** battles on
Route 3. Defeat sets the trainer's event flag AND calls `HideObject`
(`EndTrainerBattle`, `home/trainers.asm`), so `CheckForEngagingTrainers` skips
them and their sprites are removed. This is DIFFERENT from the Pewter gym Cool
Trainer (S7-8), whose defeat flag is set only by Brock's victory script — Route
3 trainers use the standard `EndTrainerBattle` path, so they stay defeated.
"Still a blocker on their tile?" is answered behaviorally: leg 2 walked
straight through the corridor with 0 battles and no blocking errors. (The
sprite ImageIndex byte read `$ff` for **every** sprite on the map after the
battles — including the never-fought Super Nerd — so that byte is not a
reliable hidden marker in this state and was not relied on.)

**5. How many trainers engaged in a single crossing, and what is the party
state after?**
**6 of 8** engaged in one eastward crossing. The two that did NOT engage are
both west-facing along row 9 — COOLTRAINER_F1 at (16,9) and YOUNGSTER4 at
(22,9); the measured path climbed to rows 4–8 early and never crossed their
sight lines. Encounter tile → trainer:

    (11,6) ← YOUNGSTER1 (10,6) RIGHT
    (14,5) ← YOUNGSTER2 (14,4) DOWN
    (19,4) ← COOLTRAINER_F2 (23,4) LEFT
    (19,6) ← YOUNGSTER3 (19,5) DOWN
    (27,6) ← YOUNGSTER5 (24,6) RIGHT
    (33,8) ← COOLTRAINER_F3 (33,10) UP

Party: lead entered Route 3 at L16 (HP 46/50) and ended at **L19 (HP 35/57)**.

### Two discrepancies found (reported, not fixed)
- **The design doc's trainer count is wrong.** SLICE8-CANDIDATES.md item 1 says
  Route 3 has "seven" trainers, but the ROM object file has **eight** OPP_
  trainer objects (the doc's own list shows eight). Measured 6 of 8 engaged.
- **The vendored decomp's map dimensions do not match this ROM.**
  `pokered/constants/map_constants.asm` says `ROUTE_3` is 35×9, but TestProbe
  on the live ROM reports **70×18** (and the player physically reached x=59).
  Do not trust the decomp's dimensions for this ROM; measure with TestProbe.

### Permanent test added (behavior passes, so it is pinned)
`TestTravelRoute3TrainerCrossing` in `skill/travel_test.go`, skipped under
-short (full journey, like `TestTravelToPewter`/`TestGymBoulderBadge`). It runs
post_errand → Brock, then crosses Route 3 east to (59,1) with bounded blackout
retries, and asserts: arrival at the destination, controllable, **≥1 trainer
battle fought**, and the lead's level increased (experience from won battles).
A regression that made Travel report ErrBlocked / a step timeout on the walk-up,
or fail to drive the battle to a result, would show up as a non-nil err or a
missing arrival. Proven PASS in 111s: "6 battle(s), lead L16 -> L19, arrived
controllable at (59,1)".

### Verified
`go build ./...`, `go vet ./skill/`, `go test ./skill -short -count=1` green
(63s), and the permanent test PASS under a full run. No `zz_*_test.go` left
behind (scratch `skill/zz_trainer_test.go` deleted); no ROM/.gb/.sav/.state
committed.

### For the next task
- This confirms Travel **survives** the ambush (fights + resumes) but does not
  **avoid** it: it pays the full battle tax for every trainer whose line it
  crosses. A small party can pay Route 3's 6-battle tax once (measured), and a
  return crossing is free only because the defeat flags stuck.
- The two unengaged trainers prove a path CAN avoid specific sight lines, so a
  sight-line-aware router has a real, measurable benefit (0 battles vs 6). That
  is the actual slice-8 question: "can the router see a sight line?" Sight
  lines are derivable from facing + range, both already in `rom.Object` after
  S7-5.
- The trainer fight verb (a Kind + an Offer branch) is still missing — S7-7
  reports trainers in `MapObjects` but does not offer them. This task
deliberately did NOT add one.
- Post-Brock / post-leg checkpoints were written to `/tmp/s83_*.state` (not
  committed) if a follow-up wants to resume without re-running the journey.

## S8-4: skill.Flee — escape a wild battle; trainer battles refuse RUN

### What landed
- `skill/flee.go`: `Flee(m *emu.Emu, attempts int) error` + `ErrTrainerBattle`.
  Selects RUN (the last entry of the FIGHT/ITEM/PKMN/RUN grid) and follows it
  to a resolution read from RAM: battle over → settle and return nil; the
  NoRunningText box → `ErrTrainerBattle`; menu up again → retry with better
  odds. Faint-and-switch episodes are answered exactly as Battle answers
  them. Not wired into Travel (out of scope for this task).
- `red/sym/addresses.go`: `NumRunAttempts uint16 = 0xD120`.

### ROM-derived facts (read, not guessed)
- **RUN is unreachable by SelectMenuItem.** The grid is 2 columns ×
  mainMenuMax+1 rows with wMaxMenuItem == 1 per column, so the item index
  tops out below RUN. `selectRunEntry` steps and verifies against
  wTopMenuItemX/wCurrentMenuItem (same pattern as SwitchActive).
- **Trainer refusal is text-only.** `TryRunningFromBattle .trainerBattle`
  (engine/battle/core.asm:1496) prints NoRunningText ("No! There's no running
  from a trainer battle!") and sets wForcePlayerToChooseMon — but its caller
  `BattleMenu_RunWasSelected` zeroes that flag in the same instruction
  sequence, so it is NOT observable. The positive signal is the text on
  screen; Flee detects the marker "running from a" (on NoRunningText only).
- **wNumRunAttempts (0xD120) increments once per WILD roll** (after the
  trainer check) and is zeroed at battle end (end_of_battle.asm:54). So the
  max sampled during a flee == attempts used, and it reads 0 after a
  trainer refusal — both assertions are positive.
- **In THIS ROM the lead outruns every reachable wild.** post_pokeballs lead
  (species 0xB1, L15→16) has speed 20–26; Route 1 wilds (species 0x24/0xa5,
  L3–5) and Route 2 wilds (same two species, L2–5) run speed 7–12. The escape
  quotient short-circuits on the speed comparison (StringCmp), so a wild flee
  succeeds on attempt 1 every time — measured 7/7 on Route 1 and 8/8 clean
  battles on Route 2. A failed wild attempt ("Can't escape!") is therefore
  unreachable from any fixture in this ROM; the only deterministic
  never-succeeds case is a trainer battle, which is exactly what
  TestFleeTrainerBattle covers.

### Permanent tests (both skipped under -short)
- `TestFleeWildBattle`: post_pokeballs → Route 1 → EnterWildBattle →
  Flee(5). Asserts the battle is over (DecodeBattle nil), the player is
  controllable, and the per-frame-sampled max of wNumRunAttempts lies in
  [1, 5] — at least one roll happened (RUN was really selected) and no more
  than `attempts` (the retry bound). Proven PASS in 1.1s: "fled in 1
  attempt(s)".
- `TestFleeTrainerBattle`: post_pokeballs → Pewter Center (heal) → gym x=1
  side corridor (the S7-8 route that stays off the Cool Trainer's sight line)
  → GoTo onto row 6 at (5,6), which engages him (pre-battle box →
  ErrDialogueInterrupted, paged with RecoverDialogue) → Flee(3). Asserts
  `errors.Is(err, ErrTrainerBattle)`, the battle still in progress, and
  wNumRunAttempts == 0 (no wild rolls; Flee did not loop attempts times
  against a wall). Proven PASS in 9.2s.

### Verified
`go build ./...`, `go vet ./skill/`, both tests green under full runs. No
`zz_*_test.go` left behind (scratch `skill/zz_spd_test.go` deleted); no
ROM/.gb/.sav/.state committed.

### For the next task
- A wild flee in this ROM is a one-try affair; the retry loop exists for
  correctness (odds improve +30/roll per attempt) but will not be exercised
  by any reachable fixture unless a future map ships faster wilds.
- `Flee` is NOT wired into Travel: a wild battle mid-route still costs the
  full Battle tax. If a future task wants "escape instead of fight" routing,
  the seam is Travel's ErrBattle interception (travel.go).
- The Cool Trainer re-arms on every crossing (his defeat flag is set only by
  Brock's victory script), so TestFleeTrainerBattle pays his full two-mon
  presence every run — cheap at L16+, but a party that has not trained yet
  should heal first, as the test does.

## S8-5: UseFieldItem — a Potion from the overworld (START -> ITEM)

### What landed
- `skill/field_item.go`: `UseFieldItem(m, item, slot)` — START -> ITEM ->
  bag entry -> party slot, every menu step-and-verify (SelectMenuItem,
  selectBagEntry, SelectPartySlot — all reused, none re-implemented).
  Postcondition from RAM via state.DecodeParty: the target's HP ROSE or its
  status byte CLEARED, else ErrFieldItemNoEffect naming both values; the bag
  count must also drop by one. Paging the result text stops at any
  DecodeTwoOptionMenu menu and returns ErrFieldItemPrompt WITHOUT pressing A
  (the S6-3/S6-4 trap). The chain closes with B, not A: after the result box
  the game returns to the BAG LIST, where A would select an entry.
- `skill/field_item_test.go`: TestUseFieldItemPotion — viridian_mart fixture,
  hidden POTION at (1,18) on map 0x33 (faced, not stepped on: hidden events
  fire on A toward the tile), Route 1 wild battle to damage the lead, then
  UseFieldItem raises the lead's HP from RAM. -short-skipped; proven PASS in
  17.9s (one blackout on the forest leg, recovered by the Travel retry).
- Not wired into Heal/Travel, no objective Kind — S8-7 decides.

### Start menu indices (measured + derived, the task's open question)
The menu GROWS with story progress; a hardcoded index is wrong after the
next flag. `startMenuShape` derives both from EVENT_GOT_POKEDEX (flag 37,
state.EventGotPokedex) instead of writing a constant:
- WITHOUT pokedex: 6 items, ITEM at index 1. MEASURED live in
  TestSelectMenuItemStartMenu (menu_test.go): wMaxMenuItem=6, cursor reaches
  5 and wraps to 0, A on index 1 opens the bag list (wListMenuID=3).
- WITH pokedex: 7 items, POKéDEX printed FIRST (index 0), ITEM shifts to
  index 2. Derived from draw_start_menu.asm: `CheckEvent EVENT_GOT_POKEDEX`
  gates the box height ($0e vs $0c) and the item count ($07 vs $06), and the
  pokedex branch prints StartMenuPokedexText before everything else. The
  S8-5 test runs from viridian_mart (post-errand, flag set), so it exercises
  the index-2 path end to end.
- Draw-readiness signal: wMaxMenuItem is the LAST write in DrawStartMenu,
  so (wFontLoaded != 0 && wMaxMenuItem == derived count) means fully drawn
  in the expected shape. Also measured: right after a battle ends, a single
  START tap can be lost — UseFieldItem re-taps until drawn (5 attempts).
- USE/TOSS prompt identification: DecodeTwoOptionMenu alone is NOT enough —
  the bag list itself decodes as a two-option menu (one entry + CANCEL row).
  useTossPrompt additionally requires the top item at tile (11,14), where
  start_sub_menus.asm .choseItem puts it.
- The "Use item on which #MON?" party menu is identified by its text marker
  (useItemPartyMenuUp in party.go, added this task); SelectPartySlot drives
  it.

### For the next task
- UseFieldItem handles heals and status cures; it does NOT guard against an
  item that cannot affect the target (e.g. a POTION on a full-HP mon fails
  ErrFieldItemNoEffect, which is the correct reading: no effect happened).
- S8-7 wires in callers; the function takes raw item id + slot, no romData.
- The lost-START-tap-after-battle behavior (wFontLoaded stuck 0 for the whole
  draw budget, menu opens 120 frames later) is a real ROM quirk measured this
  task; any future code that taps START once after a battle will hit it.

## S8-6: nine destinations + does the router route Route 3 → Mt Moon → Route 4?

Add nine place-table entries and measure whether the router can cross Route 3's
north seam into Route 4 and then through Mt. Moon's ladders to Route 4 — the
first destination chain to cross a multi-floor indoor dungeon. No verb, no
objective Kind; the only test beyond the table check is the routing measurement.

### The nine destinations (all probed: walkable + reachable)
Added to `skill/goto.go`; every coordinate verified with TestProbe against the
live ROM (tile walkable, and on a component that touches a seam/entrance so it
is actually reachable, not just standable):

    name                    map   tile      note
    route 3                 0x0e  (59,1)    past every trainer; N edge -> Route 4
    route 4                 0x0f  (10,10)   comp 2: cave entrance + PC + S seam
    mt moon 1f              0x3b  (20,18)   open floor, reachable from (14,35)
    mt moon b1f             0x3c  (14,16)   open floor below the ladder drop
    mt moon b2f             0x3d  (20,18)   deepest floor, ladder landing area
    mt moon pokemon center  0x44  (3,3)     interior room, warp 0 from Route 4 (11,5)
    cerulean city           0x03  (5,18)    W seam (rows 12,13,18,19,21..35)
    cerulean pokemon center 0x40  (3,3)     interior room, warp 0 from Cerulean (19,17)
    cerulean gym            0x41  (4,3)     interior room, warp 0 from Cerulean (30,19)

`TestS86NewDestinations` pins map id + tile per name; `TestPlaceDestinationsStandable`
(already present) now also confirms each is walkable on its own collision grid.

### The router measurement: it routes through the cave
Pinned by `TestRouteThroughMtMoon` (deterministic, graph-level, no emulator).
The descent **skips 1F** — Route 4 has a direct warp (24,5) to B1F, so the
shortest path never touches the 1F floor:

    down: 0x0e -north edge-> 0x0f  -warp(24,5)-> 0x3c  -ladder(17,11)-> 0x3d
    up:   0x3d -ladder(25,9)-> 0x3c  -warp(27,3)-> 0x0f

1F is connected separately: Route 4 warp (18,5) → 0x3b, and 0x3b ladders down
to 0x3c. So the router answers "yes" to Route 3 → through Mt Moon → Route 4.

### Route 4 is a 5-component map (why the B1F exit is a trap)
Route 4's collision grid splits into five walkable components. Component 2
holds the cave entrance (18,5), the PC (11,5), and the south seam to Route 3;
component 0 is **sealed** (no edge, no warp-out) and contains the B1F exit
landing (24,5) and a pocket at (60,8). The graph edge B1F warp (27,3) → Route 4
therefore lands the walker in component 0, from which there is no walkable path
back to component 2. This is why the descent uses the direct (24,5)→B1F warp
down but the only clean return the router knows is B2F→B1F→(27,3)→Route 4 — a
round trip that dead-ends in the sealed pocket. A full walk of the ascent is
therefore not currently executable; the graph-level route (above) is the honest
measurement and what the test pins.

### FINDING (not on this task's surface): a collision-grid defect on Mt Moon 1F
A full emulator WALK down to B2F fails deterministically on Mt Moon 1F: the
intra-map BFS routes through grid cell (10,22)→(9,22), and the ROM blocks the
step onto (9,22). Two independent runs failed at the exact same tile; the
failure dump shows no sprite at (9,22), `wJoyIgnore` = 0, player controllable.
So `world.Build` marks (9,22) walkable when the ROM treats it as a wall.

Root cause is in `world/grid.go`'s Build: each 2×2 step's walkability is read
from a single "bottom-left" sub-tile (`romData[tilesOff+(2*sy+1)*4+2*sx]`, a
heuristic measured on Oak's Lab). On Mt Moon 1F that heuristic picks a
walkable sub-tile for the step at grid (9,22) while another sub-tile of the
same step is a wall, so the game refuses the step the walker plans. This is a
localized grid defect (no prior test walked Mt Moon 1F, so it never surfaced),
not a routing failure — the router's route is correct. Tracked separately; do
not adopt here.

### For the next task
- The nine destinations are in the place table and probed; any objective or
  journey can now name them.
- A full B2F walk (and any Mt Moon 1F traversal) needs the `world.Build`
  single-sub-tile collision heuristic fixed for mixed-collision steps — that is
  the follow-up this task hands back.
- Route 4's sealed component 0 means "exit the cave and re-enter" is not a
  walkable round trip; plan Mt Moon excursions as one-way descents or fix the
  grid so the B1F exit lands on a live component.

## S8-7: fight/flee policy in Travel + the Talk seam (TestCeruleanJourney)

### What landed

**`skill/travel.go` — an opt-in fight/flee policy.** `travel()` now takes a
`resolveBattle` strategy; `Travel` keeps its old behaviour (a new `fightOnly`
resolver produces byte-identical messages and counter semantics, so every
existing measurement is unchanged), and a new exported `TravelFlee` passes
`fleeThenFight`: it RUNs out of wild battles and fights trainers (which refuse
RUN — S8-4). `TravelResult` gained `Flees int`, and the engagement bound now
counts flees + fights, so "bounded" holds for both policies. The policy is the
`fleeThenFight` resolver (flee first, fight only when `Flee` returns
`ErrTrainerBattle`); its two halves are proven by `TestFleeWildBattle` (a wild
is fled) and `TestFleeTrainerBattle` (a trainer refuses RUN), and end-to-end a
post_starter → Viridian leg fled its one wild (`Flees=1 Battles=0`) instead of
fighting it, while `TestCeruleanJourney` flees 9 wilds / fights 0 across the
forest legs.

**The Talk seam.** An NPC's line reaches the per-frame sampler through
`skill.Talk` with an `OnFrame` hook installed — proven in isolation on a Route
3 state (the Super Nerd's "That tunnel from CERULEAN…" was captured by the
tape), and wired into `TestCeruleanJourney` as a HARD assert for whenever the
journey actually reaches Route 3. The NPC is identified by `Kind == "person"`
(the sole plain NPC on Route 3, at (57,11)); coordinates come from
`agent.MapObjects`, never a literal.

**`TestCeruleanJourney`** (skill/journey_test.go) reuses the S7-8 scaffold. It
travels with `TravelFlee`, trains the lead to L12, beats Brock if it can get
there, and — if Route 3 is reached — proves the seam. It does NOT hard-fail on
a leg blocked by a pre-existing issue: it logs where it stopped and hands the
blocker back (see below). On this build it flees 9 wilds, fights 0, trains to
L12 in 19 battles, then stops at the forest north gate on the Youngster
stalemate and reports the Cerulean finding. It passes by documenting reality,
not by forcing a pass.

### FINDING (pre-existing, handed back): the S8-6 grid defect is broader than Mt Moon
The `world.Build` single-sub-tile collision heuristic (S8-6 found it on Mt
Moon 1F) fragments **more** maps, and it is what actually blocks this journey:

- **Route 4** — already known: the Route 3 → Route 4 seam lands in a 159-tile
  west component whose max x is 19, so it cannot reach Cerulean's east exit
  (x=89). Cerulean is therefore unreachable via Travel.
- **Route 2 (0x0d)** and the **Viridian Forest (0x33)** — measured the same
  way: `PROBE_AT` tiles that are individually walkable report "no reachable
  walkable tile on the … edge" / "no path" between interior points. So a
  Viridian → Pewter leg that should go straight up Route 2 instead detours
  through the forest, and the forest's own north-south crossing is not one
  connected component.

Fixing `world.Build` for mixed-collision steps (the S8-6 follow-up) is what
unblocks all of these at once; it is not this slice's job.

### FINDING (game answering, not a defect): the (2,18) Youngster stalemate
The post_errand lead (species 177) cannot beat the Viridian Forest (2,18)
Youngster's mon (species 112). It looped to `Battle`'s 60000-frame cap in two
independent runs — once with its moves PP-exhausted and once with full PP — so
it is a genuine type-ineffective stalemate, not move exhaustion. Any forest
path that crosses that trainer (and, given the fragmentation above, every
route to Pewter does) dead-ends there. This is the ROM answering, not a bug in
this slice; a different lead or a PP/type-aware battle policy would clear it,
but neither is on this task's surface.

### Confirmed gate: Pewter's east exit needs Brock
`PROBE_BLOCK` with the four "take on Brock!" tiles (35,17)/(36,17)/(37,18)/(37,19)
occupied shows no path to Pewter's east edge until `EVENT_BEAT_BROCK` is set.
So beating Brock is a real prerequisite for leaving Pewter east toward Route 3
— the journey cannot shortcut it.

### For the next task
- Fix `world.Build`'s single-sub-tile heuristic for mixed-collision steps; that
  alone makes Route 2, Route 4 and the forest one connected component each and
  unblocks Cerulean via Travel.
- If the journey is to clear the forest, it needs a lead (or battle policy) that
  can beat species 112 — the post_errand species-177 lead stalemates it by type.
- The flee policy and the Talk-seam proof are done and reusable; a future
  Cerulean milestone only needs the grid fix to arrive.

## S8-9: world.Build's single-sub-tile rule is CORRECT — the S8-6/S8-7 "defect" is a misdiagnosis

Task premise: `Build` "decides walkability from a single sub-tile (bottom-left),
fragmenting Route 2, Route 4, Viridian Forest and Mt Moon 1F into disconnected
components," fixable by testing a different sub-tile. **That premise is wrong,
and it is now proven, not guessed.** No change was made to `world/grid.go`.

### The rule is bottom-left, settled by the decompilation (not inferred)
`TryWalking` (`pokered/engine/overworld/movement.asm`): `GetTileSpriteStandsOn`
returns a pointer to **the lower-left tile of the 2×2 block the sprite stands on**
(the comment in the source says so verbatim). Each direction handler then shifts
that pointer by exactly ±2 tiles (`.moveDown`/`.moveUp`: `±2*SCREEN_WIDTH`; 
`.moveLeft`/`.moveRight`: `dec hl; dec hl` / `inc hl; inc hl`) to the destination
block's lower-left tile, and `TryWalking` does `ld c, [hl]` → `call CanWalkOntoTile`.
`CanWalkOntoTile` checks that ONE metatile (`c`) against `wTilesetCollisionPtr`
's walkable list and performs NO other tile-collision test. So the ROM's
tile-collision rule is: **is the destination block's lower-left sub-tile in the
walkable list?** That is byte-for-byte what `Build` reads:
`romData[tilesOff+(2*sy+1)*4+2*sx]`.

### Confirmed on real hardware (Pallet Town, tileset 0)
Two cells with opposite sub-tile patterns; only BL is consistent with what the
player actually did (fresh fixture load per cell to avoid step state-bleed):
- (10,14): TL=T TR=T **BL=T** BR=F → player STOOD there (walkable). Rules out
  BR and BL∧BR ("both bottom").
- (7,14): TL=T TR=T **BL=F** BR=T → player BLOCKED. Rules out BR, TL, TR.
- Intersection = {BL}. The scratch `getSub` extraction matched `Build`'s index
  exactly, so this is a measurement of the game, not of the extractor.

### The codebase already pins this rule
`TestBuildOaksLabCollision` (world/grid_test.go) asserts the (6,4)/(6,3)
regression pair "fails under the old top-left indexing (2*sy), passes under the
bottom-left indexing (2*sy+1)" — i.e. the suite ALREADY encodes BL as correct.
Changing `Build` to BR or BL∧BR would break that passing test AND make the grid
disagree with the ROM. There is no rule change that both fixes the alleged
fragmentation and keeps the existing tests green.

### The S8-6 Mt Moon 1F (9,22) "defect" does not reproduce as a tile issue
- Mt Moon 1F is **40×36** in this ROM (`PROBE_MAP=0x3b`); the vendored
  `map_constants.asm` says 20×18 — stale, the same class of error S8-4 found
  for Route 3. Do not trust decomp dimensions; probe.
- Grid window at (10,22): `(8,22)=#`, **(9,22)=.**, `(10,22)=@`. The ROM rule
  passes the step onto (9,22) (its BL sub-tile is on the walkable list).
- The only nearby object is the Youngster at **(7,22)** (object file +
  `PROBE_STATE` agree), behind the (8,22) wall and 2 tiles from the destination
  — its 2×2 does not overlap the player's destination block, so
  `DetectCollisionBetweenSprites` cannot block that step.
- So (9,22) is genuinely walkable and unblocked; the grid marks it correctly.
  S8-6's "ROM blocks the step onto (9,22)" was a different cause (the (8,22)
  wall one tile further, a trainer engagement, or a BFS path that went
  elsewhere), not a sub-tile collision defect.

### The "fragmentation" is real geometry that NO sub-tile rule can fix
Measured the barrier rows/columns under BL, BR, and any-of-4 (all four
sub-tiles walkable):
- **Route 2 (0x0d), 20×72:** rows y=36,37 are non-walkable across the FULL
  width (x=0..19) under every rule. Raw tiles there: left half `0x40`, right
  half `4c/53` and `5a/12` — none on the walkable list. This is the lake: a
  surf / warp-structure crossing (warps at (16,35) and (15,39) bracket it), not
  a walkable path. North edge → Pewter (0x02), south edge → Viridian (0x01).
  The north/south split is genuine on-foot geometry.
- **Route 4 (0x0f), 90×18:** columns x=20..23 are solid across the FULL height
  (y=0..17) under every rule. West (x≤19) and east (x≥24) are genuinely
  separate regions.
- **Viridian Forest (0x33):** **1 component under BL** (N↔S reachable —
  correct); **109 components under BR**. So BL is the rule that matches the
  game; switching to BR would be a massive regression, not a fix.

Because both Route 2's and Route 4's barriers are solid under EVERY sub-tile
rule, the proposed fix (test a different sub-tile) cannot address either map.
The multi-component structure is real in-game geometry, not a heuristic artifact.

### Determination
`world/grid.go Build` correctly implements the ROM's collision rule (the
destination block's lower-left sub-tile), proven by decompilation, confirmed on
real hardware, and already pinned by `TestBuildOaksLabCollision`. The maps cited
as "fragmented" are fragmented by genuine terrain (Route 2's surf lake, Route
4's mountain wall) that no sub-tile choice bridges, and the Mt Moon (9,22) case
is a walkable, unblocked tile that S8-6 misattributed. `TestGymJourneyAffordances`
passes with the current code. **No change made; no map ID or tileset
special-cased.**

### For the next task
- If the router must cross Route 2's lake, that is a **surf action** (a verb),
  not a grid edit — the grid correctly says you cannot WALK on water.
- Route 4's west and east halves are separate regions connected only via other
  routes/warps; model that in the graph layer, never by fudging the collision
  grid.
- If a future agent re-measures "the player got stuck at X" on a walkable tile,
  check `DetectCollisionBetweenSprites` (an adjacent NPC's 2×2) and the tile one
  step further before blaming the sub-tile rule — that is what S8-6 hit on
  Mt Moon.

## S8-8: wall triage — group the failures so 200 runs read as four bugs

### What landed (cmd/pokewall only; no emulator, no ROM, no fixtures)
- `wall.go`: `normalizeDetail` — `0x[0-9a-fA-F]+` → `<hex>`, digit runs →
  `<n>`, then capped at 128 chars. `triageGroups(order, tiles)` groups
  finished runs whose reason is error/lost AND whose detail is non-empty,
  most frequent first; each group carries the pattern, exact count, ONE
  verbatim example detail, and run ids capped at 5. New read-only endpoint
  `GET /v1/triage` (JSON) and a "failure groups" table on the grid page
  under the run table (same template, no second page). REPORTS ONLY: no
  task creation, no runner calls, no writes outside the wall's own state.
- `triage_test.go`: table test — coordinate/hex variants group; the
  connection-edge error and the text-box error do NOT group (the assertion
  that catches over-normalising); count-descending order; empty-detail run
  in no group; example is one of the inputs unmodified; id cap; endpoint +
  grid rendering. All green, `go build`/`go vet` clean.

### Worked example: the 16-run wall of 2026-08-29 reads as five groups
One curl of /v1/dashboard, details normalised and counted:

    9  agent: go to <place>: skill: GoTo: skill: Traverse: walk to warp on map <hex>: text box interrupted movement
    3  agent: go to pallet town: skill: GoTo: skill: Traverse: connection edge <n>c-><n> via south did not cross within <n> frames
    1  agent: go to pallet town: skill: Travel: blacked out
    1  agent: take a starter: ... rival battle result = <n>, want win
    1  unknown destination "pallet"

Nine of sixteen runs were ONE bug (the Route 1 edge, landed in S8-3); the
16th run finished with an empty detail and appears in no group. Read
run-by-run the same wall looked like sixteen separate disasters and the
dominant bug was invisible. This is what /v1/triage now prints.

### For the next task
- The grid's pattern column renders `<hex>`/`<n>` (HTML-escaped in source);
  the verbatim example column carries the real coordinates/map ids — that
  column is what makes a group actionable.
- Triage groups finished runs only; a run still burning its retry budget
  with repeated identical failures will not show up until it settles.

## S9-1: planner can choose to flee a journey's wild encounters

### What landed
- `agent/objective.go`: `Objective.Flee bool`. `String()` renders "go to X,
  fleeing wild battles" / "heal the party at X, fleeing wild battles" when
  set. `Execute` calls `skill.TravelFlee` (same maxBattles=20 bound) instead
  of `skill.Travel` when Flee is set — KindGoTo and KindHeal's travel leg.
  Flee is NOT the default: a fled wild is XP the run did not get.
- `agent/planner.go`: `ReplyArgs.Flee *bool`; `WithArgs` applies it only to
  KindGoTo and KindHeal-with-Place (a heal in place walks nowhere), else the
  "does not apply" typed error, validated before anything lands (unmutated
  on rejection, as the table test asserts).
- `agent/llm.go`: `"flee": {"type":"boolean"}` in choiceSchema; choiceReply
  carries it through to WithArgs; system prompt now names "flee".

### Tests
- planner_test: TestWithArgsApplies (flee on goto + heal-with-place; false
  is a no-op) and TestWithArgsRejects (+3 cases: flee on train/catch/heal-in-
  place, all "does not apply", objective unmutated).
- objective_test: TestString both forms; `TestExecuteGoToFleesWildEncounters`
  (-short-skipped): leg 1 post_starter -> Viridian City via TravelFlee asserts
  Flees>0 AND Battles==0 (S8-7's measured one-wild leg); leg 2 Viridian ->
  Route 1 via `Execute{Flee:true}` asserts arrival on 0x0c. Proven PASS twice
  (1.5s each, deterministic fixture replay).

### Verified
`go build ./...`, `go vet ./agent/`, gate greps (TravelFlee in objective.go
= 3, "flee" in llm.go = 3), `go test ./... -short -count=1` green, full
`go test ./agent -count=1` with ROM green (88s). No zz_* left, no .state/.gb
committed.

### For the next task
- The e2e leg's counters come from a direct TravelFlee call: Execute returns
  only error, so a future "did it actually flee?" assertion on an Execute
  leg needs Execute to surface its TravelResult (or a log line).
- Viridian City -> Route 1 crossed clean in this replay (the S7-6 plaza
  walkers did not block); fixture replays are deterministic, so it stays
  clean.
- `skill/zz_train_dynamics_test.go` is tracked on main (committed by an
  earlier task) despite the zz_ name; not deleted here.

## S9-2: KindUseItem — the planner can heal without walking to a Center

### What landed
- `agent/objective.go`: `KindUseItem` appended after KindPickup (no renumber).
  `Objective.Slot int` (0-based, the exact addressing `skill.UseFieldItem(m,
  item uint8, slot int)` takes — no second scheme). Reuses `Objective.Item`.
  Validate: unknown item id and slot outside 0..5 are typed errors, never
  clamped. String: "use a POTION on party slot 0" (article helper for the
  vowel items). Execute wraps skill.UseFieldItem's error with the objective;
  the HP-ROSE / status-CLEARED postcondition lives in the skill (S8-5).
- `agent/offer.go`: Offer appends one use-item objective per (bag medicine,
  slot it would do something to), right after the heal block. `partyHurt`
  factored into per-mon `monHurt`; HP healers want a hurt-but-alive mon
  (fainted excluded: ItemUseMedicine refuses a potion on a fainted mon with
  .healingItemNoEffect), status cures want the matching status name.
- No planner.go/llm.go change needed: Offer hands out concrete Item+Slot
  objectives, so the model picks by index; a bare {"choice": N} selects one
  unchanged.

### The "field-usable items" table and why it is written down, not parsed
`fieldMedStatus` (agent/offer.go) maps ten item names to "" (HP healer) or a
status name. Every entry is DERIVED from the vendored decomp, not typed:
ItemUsePtrTable (pokered/engine/items/item_effects.asm) sends exactly these
ten to ItemUseMedicine, and .checkItemType is the per-item rule (POTION..
FULL_RESTORE → .healHP; ANTIDOTE/BURN_HEAL/ICE_HEAL/AWAKENING/PARLYZ_HEAL →
.cureStatusAilment with masks PSN/BRN/FRZ/SLP_MASK/PAR). The ROM route
(parse ItemUsePtrTable from ROM bytes at runtime) was NOT taken: Offer works
on the decoded Observation with no romData, and the codebase precedent for
argument vocabulary is decode-once-write-down-with-citation (speciesTable,
itemTable — the "farow" incident). Each entry here cites its dispatch line.

### Tests
- objective_test: TestString (both article forms), TestValidateRejects
  (unknown item, slot -1 and 6 rejected; slots 0 and 5 accepted), and
  TestExecuteUseItemHealsTheTarget (-short-skipped): viridian_mart fixture →
  hidden POTION at (1,18) on 0x33 (S8-5's setup) → Route 1 damage battle →
  Execute KindUseItem → asserts from RAM that lead HP ROSE and the bag count
  dropped. Proven PASS in 17.7s (one blackout on the forest leg, recovered).
- offer_test: four TestOfferTable cases — hurt party + potion offered; whole
  party + potion NOT offered; hurt party, empty bag NOT offered; poisoned
  whole-HP mon + antidote offered (status cure independent of HP).

### Verified
`go build ./...`, `go vet ./agent/`, `go test ./... -short -count=1` green,
full `go test ./agent -count=1` with ROM green (105s). No zz_* left, no
.gb/.sav/.state committed.

### For the next task
- A UseItem objective offered inside a Center is legal but wasteful (the
  nurse is free); Offer does not filter on location by design ("possible,
  never wise"). If runs show models burning potions in Centers, that is the
  gate to add — and it is a judgement gate, flag it as such.
- UseFieldItem cannot revive: a fainted mon is refused by the ROM, so the
  offer excludes fainted slots for HP healers. A Revive objective (different
  item class, .healHP path with the revive branch) would be a new kind.

## S9-3: ROAD-TO-ELITE-FOUR.md — survey of the Cerulean → Indigo road

Recovery run: the previous attempt left a scratch test but no document.
This run re-measured everything (scratch `agent/zz_road_test.go`, deleted
before commit; output to a temp file, never into context) and wrote the
deliverable.

### What landed
- `docs/ROAD-TO-ELITE-FOUR.md`: the 8-leg road with per-leg status, the
  five gates (G1 Route 2 wall / forest crossing, G2 Flash at Route 2 Gate,
  G3 Boulder badge at Route 22 gate `0xC1`, G4 Surf + seven badges on
  Route 23, G5 none at Indigo), the badge ledger, the Surf acquisition
  chain (Warden's House is inside Fuchsia City in this ROM), and the Q3/Q4/
  Q5 answers: map-level graph routes it (8 legs, 2 phantom edges, 1 water
  gate); verb gaps are Surf/HM-use and an Elite Four battle kind plus
  routing gaps for phantom edges and the dead-end gate; slice count seven
  (floor five, ceiling eight).

### Findings that surprised
- Route 2's row-22 wall is not water — solid `$50`/`$3D`. The crossing is
  the Viridian Forest through gate buildings `0x2F`/`0x32` (130-step path,
  measured). The agent already did this in a live run.
- Route 23's three full-width bands (rows 81/92/101) are water + cliff
  tiles; PLATEAU's walkable list is six tiles, so Surf is the only crossing.
  Walk-only BFS from the gate exit cannot reach the north edge — the water
  is what partitions the map.
- The vendored decomp is of THIS ROM (Giovanni/Viridian/Earth badge are in
  it), so all script citations are valid here; gym→badge table confirmed
  against `data/maps/badge_maps.asm`.
- Map `0x0B` (UNUSED_MAP_0B) has warp entries pointing at Indigo but an
  invalid header — dead data, noted in the doc.

### Verified
`go build ./...` green after scratch deletion; all cited
`pokered/<file>:<line>` spot-checked against the vendored tree (identical
to `~/.cache/pokered`); no `.gb`/`.sav`/`.state` committed; no `zz_*` left.

## S9-4: the run keeps what it is trying to do, across rounds

### What landed (one string and an integer — no plan DSL)
- `agent/planner.go`: `IntentCap = 200` bytes, typed `ErrIntentTooLong`.
  `ReplyArgs.Intent` applies to EVERY kind in `WithArgs` (unlike
  level/species/item); over-cap is a typed REJECTION of the whole reply —
  never a truncation — with the objective returned unmutated.
- `agent/objective.go`: `Objective.Intent string`. Run memory, not an
  argument: Validate/Execute ignore it, String() does not render it, so
  Knowledge.Done (keyed on String()) is unaffected.
- `agent/observe.go`: `Observation.Intent` + `IntentAge int`, set by Run,
  never decoded from RAM.
- `agent/run.go`: Run carries the most recent non-empty intent and its age.
  A different non-empty intent replaces it (age 0); the same sentence or
  silence ages it by one round. Run NEVER writes/edits/summarises the
  sentence — that would be planning for the model again.
- `agent/llm.go`: `intent` in choiceSchema (description states the 200-byte
  cap) and in choiceReply; the system prompt now tells the model the field
  exists, will be read back as Intent/IntentAge, and to change it only when
  its purpose changes.

### Tests (deliverable = the round trip)
- `TestRunCarriesIntentAcrossRounds`: scripted 3-round run; obs on round N
  carries what round N-1 sent verbatim, age counts up while unchanged (0 ->
  1) and resets to 0 when it changes. PASS in 6.5s.
- planner_test: intent applies to any kind alongside other args; exactly
  200 bytes accepted, 201 rejected with errors.Is(err, ErrIntentTooLong),
  objective unmutated.
- llm_test (httptest): captured request body carries `"Intent"`/`"IntentAge"`
  and the system prompt names the field; a schema reply's intent lands on the
  returned objective; an over-cap reply is rejected typed, not cut.

### Verified
`go build ./...`, `go vet ./agent`, `go test ./agent -count=1` (117s) and
`go test ./... -short -count=1` all green (ROM + shared fixture dir). No
zz_* files; nothing committed.

### For the next task (S9-7 measurement)
- The system prompt CHANGED: every prior badgerun row was scored without the
  intent sentence in the prompt and without Intent/IntentAge in the
  observation JSON (~20 extra tokens/round). Not comparable side by side.
- If the model ignores the intent field live (empty on purpose), that is a
  FINDING to write down — do not make Run synthesise an intent; that defeats
  S9-7's measurement. An unchanged intent is a log finding, not a stop
  condition: no Stop reason was added.

## S9-5: a resumed run is not amnesiac

### What landed
- `agent/memory.go`: versioned serialisable Knowledge. `memoryFile` holds
  Visited, Places, Completed, Talked + S9-4's intent and age — NOT Adjacency
  (route geometry, rebuilt from the ROM by world.BuildGraph every run) and
  NOT Observation/History/offered (re-derived in one round). Filename is
  versioned: `<state-base>.knowledge-v1.json`. `writeMemoryFile` is atomic
  (temp+rename); `LoadCheckpointMemory(statePath, adjacency, log)` derives
  the knowledge path FROM the state path — there is no API that takes a bare
  knowledge path, so the pairing is structural. Wrong version, truncation,
  garbage, missing file, and over-cap intent all return an EMPTY
  ResumedMemory + one log line: clean start, never a partial load, never a
  panic.
- `agent/run.go`: `Budget.ResumeFrom` (a checkpoint .state path). Run reads
  the state, `m.LoadState`s it, and loads the paired knowledge from that ONE
  path before round 1 — a plain Run (empty ResumeFrom) is unchanged.
  `checkpointRing.write` now writes the knowledge beside each state in the
  same call (same base name, so they cannot drift); `evict` evicts pairs as
  one and drops orphaned knowledge (knowledge with no state is exactly the
  "knows things this save has not seen" case).
- `agent/offer.go`: `Knowledge.restore(mem)` — read-side half; touches only
  game-shown fields, leaves Adjacency as built.

### Tests
- memory_test.go (internal pkg): round trip with every field + intent/age
  intact, Adjacency from the argument not the file; wrong-version /
  truncated / garbage each assert a CLEAN EMPTY START + log line; missing
  file and over-cap intent too.
- run_test.go: `TestRunResumesKnowledge` — first run talks to (2,1) on
  Pallet Town then completes a second objective (so the LAST checkpoint was
  taken with the talk already in knowledge); a FRESH emulator resumes from
  the last checkpoint and its Offer does NOT re-offer the talk, while the
  first run's Offer did. (2,1) was measured to stay offered on the post-talk
  state with EMPTY knowledge — the (6,3) NPC hides after a talk and would
  let the test pass for the wrong reason; that trap is in the test comment.
- Pre-existing `TestRunCheckpointRoundTrips` / `TestRunCheckpointRingIsBounded`
  updated: a checkpoint is now a state+knowledge pair (helpers match
  `.state` only; the ring test asserts two states each with its knowledge
  beside it).

### Verified
`go build ./...`, `go vet`, full `go test ./agent -count=1` green (115s,
ROM + shared fixture dir), `go test ./... -short -count=1` green. No zz_*
left (skill/zz_train_dynamics_test.go is tracked on main, untouched); no
.state/.gb committed; edits left uncommitted for the runner.

### For the next task
- badgerun/pokewall do not resume yet: the seam is `Budget.ResumeFrom` +
  the newest `.state` in the run's `checkpoints/` dir (lexicographic max).
- Resumed round numbers restart at 1; old ring entries stay until evicted
  (frame numbers in the names keep them distinct). If a resumed run writes
  into the same ring and that confuses tooling, offset the round counter.

## S9-6: requirement harvesting — the run keeps what the game told it it can't do

### What landed
- `agent/offer.go`: `Knowledge.Requirements []string` — the raw sentences,
  verbatim, newest first, capped. `HeardRequirement(line)` is the ONLY writer:
  it trims, dedups (case-insensitive), prepends, and caps at `requirementCap`.
  `SawDialogue` now runs each settled line through `looksLikeRequirement`
  before storing it. The filter is a whole-phrase substring list —
  `you don't have`, `you need`, `only if you have`, `can't go through` — each
  verified against a real ROM text in the comment above it. Matching runs on
  whitespace-normalised text (Gen 1 wraps at the line width, so "You can't
  go\nthrough here!" must match "can't go through"), but the STORED value stays
  verbatim. No badge/item names are parsed; there is no struct, no branch in
  Offer — Offer never reads Requirements (grep-verified).
- `agent/observe.go`: `Observation.Requirements []string`, initialised to an
  empty slice so it serialises as `[]` not `null`.
- `agent/run.go`: after `noteObservation`, copies `known.Requirements` into the
  observation (defensive copy) so the planner sees what the game said.
- `agent/memory.go`: `memoryVersion` bumped 1→2; `memoryFile` gains
  `Requirements`; encode writes it, restore re-validates each line through
  `HeardRequirement` (keeps the field honest if the shape list ever shrinks).
  Old v1 files are rejected as wrong-version → clean empty start.

### The shapes are a FILTER, not knowledge
They decide which lines to carry forward, never what is true. A line that
matches a shape is stored verbatim and shown to the planner; the planner (or a
human reading the log) decides what it means. That is the whole design: the ROM
states requirements in a handful of idioms, we catch those idioms, and we do
not try to understand them.

### Tests
- offer_test.go (unit): `TestKnowledgeHarvestsStatedRequirements` feeds the
  REAL Route 23 guard text (`pokered/text/Route23.asm`: "You can pass here /
  only if you have / the CASCADEBADGE!" and "You don't have the / CASCADEBADGE
  yet!") plus chatter, asserts the two requirement lines are harvested verbatim
  (chatter is not), that re-hearing dedups, and that a plain line never matches.
  `TestKnowledgeRequirementShapesAndCap` pins each shape to a ROM example and
  checks the cap + newest-first ordering.
- memory_test.go: `TestMemoryRoundTrip` now feeds one harvested wall beside
  chatter and asserts the wall survives write→read verbatim while the chatter
  does not — the persist-across-rounds half. `assertEmptyKnowledge` covers the
  field in the clean-start cases.

### The live end-to-end proof is PENDING, and here is exactly why
The feature is proven by the unit tests above (filter on real ROM text, dedup,
cap, ordering, SawDialogue wiring, checkpoint round-trip). A live test that
drives a REAL Talk through the per-frame tape into `Observation.Requirements`
is NOT robustly writable in this build, because every reachable requirement /
wall line is blocked by something that is not this task's surface:

- The clean requirement lines are far away. The Route 23 guard and the Viridian
  Forest North Gate Super Nerd ("You need to look everywhere...") sit behind
  two documented pre-existing blockers: the (2,18) Youngster stalemate in the
  forest (S8-7) and the Route 2 world.Build fragmentation (S8-6/S8-9). Reaching
  them means adopting someone else's slice — exactly what AGENTS.md warns
  against.
- The near wall line (Old Man Sleepy, Viridian City (18,9), "You can't go
  through here! This is private property!") is in the post_errand fixture's own
  city, but his SPRITE_GAMBLER_ASLEEP object is not rendered as a live sprite in
  that state (a scratch probe decoded only slots 4 and 7; slot 5 is absent), so
  `skill.TalkAt` gets "A did not open a text box". That is a fixture/ROM quirk,
  not the tape or the harvest.

So: when S8-6/S8-7 land (or the sprite quirk is understood), the live test is
one objective — `{Kind: KindTalk}` at the North Gate Super Nerd or the Route 23
guard — and a `strings.Contains(line, "you need")` assert on
`res.Final.Requirements`. The wiring it would exercise (tape.recent →
RecentDialogue → SawDialogue → Knowledge.Requirements → Observation.Requirements)
is already covered piece by piece by the unit tests.

### Verified
`go build ./...`, `go vet ./agent ./skill`, `go test ./agent ./skill ./red/...
-short -count=1` green. Live `TestRunCheckpointRoundTrips`,
`TestRunResumesKnowledge`, `TestRunCarriesIntentAcrossRounds` green (ROM +
shared fixture dir) — confirms the memory version bump and the Run-loop
Requirements copy did not break checkpoint persistence. No zz_* left; no
.state/.gb committed; edits left uncommitted for the runner.

### For the next task
- Add the live test once a requirement line is reachable (see above); it is a
  few lines on top of the existing capturePlanner harness.
- If more requirement idioms show up in the wild, extend `requirementShapes`
  with a verified ROM example per shape. Resist adding shapes to force a match
  with an unreachable line — that is overfitting the filter to a test.

## S9-8: why a grind walk goes 141 legs without a fight (measurement)

Answer, measured: **not a phase lock — a deterministic drought window.** The
encounter roll is `hRandomAdd` (HRAM 0xFFD3) compared against the map's grass
rate; `hRandomAdd` advances by rDIV every frame, so the roll at each step is a
fixed function of the absolute frame count. A ping-pong grind samples that
fixed sequence at a constant stride, and some 141-leg windows of it contain
only 0–2 values below the rate. The journey's phase landed in one.

### The numbered account (measured 2026-08-30, real ROM, post_errand state)
1. **Legs vs encounters.** My deliberate repro on map 0x33: pair A
   ((18,40)↔(18,39)) ran 38 legs with encounters at legs 14, 27, 37 (the 3rd
   battle lost → blackout); pair B ((18,40)↔(18,34), dist 6) then ran 40 legs
   + the return walk with 10 battles. Whole run: 247 grass steps, 11 rolls
   under rate (9 fired; 2 suppressed), max dry streak 59.
2. **The walk.** `grindPair` picks adjacent cells a=(18,40), b=(18,39): one
   step per leg, **exactly 17 frames per leg** (every leg measured +17), so
   exactly one roll per leg.
3. **The roll.** After each completed step, `InitBattle` (core.asm:6664) calls
   `TryDoWildEncounter` (engine/battle/wild_encounters.asm): if the
   half-block's bottom-right tile is the grass tile, it compares
   `hRandomAdd` (0xFFD3) against `wGrassRate` (0xD887) — encounter iff
   `hRandomAdd < rate`. Map 0x33: **rate = 8/256 = 3.125% per step**, 5
   species (ids 112, 113, 124, 123, 84). `hRandomAdd += rDIV` at every vblank
   (home/vblank.asm → Random_, engine/math/random.asm) — the roll is NOT an
   independent draw. Also: `wNumberOfNoRandomBattleStepsLeft = 3` suppresses
   rolls for 3 steps after a battle/map entry (EnterMap, home/overworld.asm).
4. **Phase-lock hypothesis: refuted as a residue lock, confirmed as a
   drought window.**
   - Per-frame `hRandomAdd` delta over ~9.5k walking frames: uniform over
     0..255 (every delta seen 11–35×) — no fixed increment, no residue class.
   - Idle scan, 16 phase offsets × 500 samples at the 17-frame stride: every
     phase hits **213–227 distinct values of 256**; minimum hit count over
     sliding 141-windows: **0** at offsets 48 and 240, **exactly 2** at
     offsets 32/64/112/160 — the journey's exact event.
   - Binomial check: P(≤2 encounters in 141 legs at 8/256) ≈ **18%**; and
     maxLegs=140 sits only ~⅕σ above the mean legs for 4 battles (128±66),
     so **~35% of sessions trip the budget at all** (P(≤3)≈0.35). The
     "no-encounter phase" is a routine event, not an anomaly.
   - Why `StepFrames(123)` works: it shifts the sampling window by 123 frames
     → a different 141-window that has ≥4 hits. It resamples; there is no
     lock to break. Varying the leg (dist-6 pair → 6 rolls/leg) changes the
     stride and the count (pair B: 5 forest encounters in 40 legs) — same
     mechanism, different subsequence.

### What changed
- `skill/train.go`: the impossible diagnostic ("its grass has no wild rate?"
  — unreachable since the zero-rate gate makes `grassCells` return nil first)
  is gone. New `NoEncounterDiagnostic(legs, battles, want, mapID, rate,
  species)` states only what is known: "no-encounter phase after %d legs:
  %d encounters (want %d battles) on map %#04x, grass rate %d/256, %d
  species in wild table". Rate/species read from the wild table.
- Retry anchors: gym_test.go, journey_test.go (×2), travel_test.go now match
  "no-encounter phase" instead of "without enough encounters".
- `skill/train_test.go`: TestNoEncounterDiagnostic pins the message content
  and asserts the impossible cause is absent. Runs under -short.

### For the next task
- The principled fix needs its own task: budget legs by expected rolls
  (want ÷ rate/256, with slack for the heavy tail) instead of the fixed
  20×maxBattles+60, or make Train vary its stride so a drought window cannot
  persist. The test's frame-shift retry stays a workaround — it works, and
  this section is why.
- `skill/zz_train_dynamics_test.go` is a committed scratch test left by an
  earlier task (5f379dd); delete it when convenient.
- Scratch for this task (`skill/zz_s98_test.go`) deleted; raw logs at
  /tmp/s98_frames.log and /tmp/s98_scan.log (not committed).

## S9-9: Train stops while the party is still alive (the retreat line)

### The measurement (why half max HP)

From `post_errand` (lone level-7 SQUIRTLE, 24/24), Route 2 (map 0x0D,
entry (8,71), 24 reachable grass cells — the route is multi-component and
the north-east pocket is unreachable from the entry), three full
grind-to-blackout cycles were logged battle by battle. The lead's HP in
the last four battles of each cycle:

- cycle 1 (battles 16–19): 20/27, 14/27, **5/27** → fainted on battle 20
- cycle 2 (battles 21–24): 23/31, 15/31, **4/31** → fainted on battle 25
- cycle 3 (battles 26–29): 26/38, 17/38, **1/38** → fainted on battle 30

The pre-fatal readings are 18.5%, 12.9% and 2.6% of max — the lead is
already below half its HP four battles before it dies, and the drop from
there is one bad hit away from zero every battle. **Half max HP**
(`retreatLineNum=1, retreatLineDen=2`) stops the session while the party
can still walk; a lower line (a third) would have let all three measured
cycles reach their fatal battle.

The line cannot prevent every blackout: one battle can always drop the
lead from just above the line to zero. TestTrainSurvivesEvolution's first
segment still blacked out (level 18, SQUIRTLE→WARTORTLE) on this run; the
line ends the *expected* death — the measured sub-20% slide — not the
variance.

### What landed

- `skill/train.go`: `TrainResult.Retreated`, sentinel `ErrTrainRetreat`,
  `retreatLineNum/Den`. The check runs at session start (a resumed party
  that is already below the line stops before it fights — a retreat does
  not respawn the player, so this is what keeps a naive retry loop from
  spinning) and after every battle. Blackout still wins when both happen
  in one battle. Doc comments updated on `Train` and `TrainResult`.
- `agent/objective.go`: KindTrain maps a retreat to
  `fmt.Errorf("agent: train the lead to level %d: %w (ended level %d)",
  o.Level, skill.ErrTrainRetreat, res.EndLevel)`. The text carries the
  level, not the battle count, so two consecutive retreats at the same
  level produce identical error strings — which is exactly what the new
  run test needs to force the same-failure-twice path.
- `agent/run.go`: `ErrTrainRetreat` joins `ErrBlackedOut` in the
  exemption from the consecutive-failure accounting (no
  `consecFailures` bump, no `lastFail*` pin). Rationale: a retreat is not
  an identical repetition — the session changed the party state (level
  and HP), so the next attempt faces a different game. A planner that
  ignores damage and keeps re-retreating at the same level is caught by
  the pre-existing `StuckAfter` path instead.

### Tests

- New `skill.TestTrainRetreatsBeforeBlackout`: post_errand → Route 2 →
  one `Train(99, 40)`. Asserts `Retreated=true`, no blackout, and the
  lead's HP read from RAM: above zero, below half max. Measured result:
  4 battles, level 7→8, lead 5/27 — the session that would have blacked
  out by battle ~10 ended alive.
- New `agent.TestRunTrainRetreatTwiceDoesNotStopTheRun`: setup grinds the
  lead below the line, then two identical `{KindTrain, Level:100}`
  objectives both stop at the start check with byte-identical error text
  (the test asserts the two outcome strings are equal — without that the
  same-failure-twice check would never fire and the test would pass even
  without the exemption). Verified negative: with the exemption removed,
  the run dies on round 2 with StopFailed.
- Adapted: `TestTrainGrindsOnRoute1` now runs a segment loop (a L6 lead
  retreats after ~3 battles; heal at the Viridian Center between
  segments). `TestTrainSurvivesEvolution` and
  `TestBattleAnswersForgetMovePrompt` heal after a retreated segment. The
  journey/gym/travel grind loops' heal condition now includes
  `res.Retreated` (their old trigger was HP < 1/3, which the retreat line
  pre-empts). `agent.TestExecuteTrainCharmanderToOfferedLevel` accepts
  either typed ending (blackout or retreat).

### Verification

- `go build ./...`, `go vet ./...`: clean.
- `go test -short ./...`: all packages ok (skill 107s, agent 95s).
- Long training tests (no -short): TestTrainGrindsOnRoute1 8.6s,
  TestTrainRetreatsBeforeBlackout 5.6s, TestBattleAnswersForgetMovePrompt
  269s, TestTrainSurvivesEvolution 170s — all pass.
- `skill/zz_retreat_test.go` (measurement scratch) deleted.

### For the next task

- The journey tests' `maxHealDetours = 6` was sized for the old damage
  cadence; the retreat line makes heal detours more frequent, so a tight
  grind may now trip the cap. If TestGymBoulderBadge or a journey test
  dies on "N heal detour(s)", raise the constant — do not lower the line.
- A planner that sees `stopped while the party was alive` should walk to
  a center (or use an item) before re-offering Train; nothing enforces
  that yet, and StuckAfter is the only backstop.

## S9-10: drive the Cerulean gym once — does the generalised Gym beat Misty? (map 0x41)

Measurement task. `skill.Gym` was generalised on 2026-08-30 (0c6c9b4) from
Brock-only to a `gyms` table; this drives it against Misty (map 0x41) and
reports the four answers. No Cerulean fixture exists, so the run drives the
whole journey: post_errand → forest grind L12 → Pewter/Brock (a hard
prerequisite — Pewter's east exit stays locked until `EVENT_BEAT_BROCK`) →
Route 3 → Route 4 → Cerulean. New `skill.TestGymCascadeBadge` sits alongside
`TestGymBoulderBadge` (`-short`-skipped).

### The four answers
**1. Did Travel carry the approach past the COOLTRAINER_F?** Not reached.
The run stopped *before* Cerulean, on Route 3 (below), so the Cerulean
approach — where the gym's COOLTRAINER_F at (2,3) faces right along row 3,
the same row as `Place("cerulean gym")` (4,3) — was never driven. From code
only: `gym.go` already anticipates it and relies on `Travel` to fight
through line-of-sight trainers, which S8-4 measured it doing on Route 3.
**2. Did the fight start, and what was the outcome?** Not reached — the run
never entered the Cerulean gym, so no Misty battle started.
**3. `wObtainedBadges` on a win (bit 1 or nothing)?** N/A — no win. The run
did set bit 0 beating Brock (`wObtainedBadges=0x01` at that point); bit 1
(Cascade) was never reached.
**4. Anything the generalised Gym assumed that is only true in Pewter?**
See below — answerable from code even without a live win.

### Where it stopped (one trainer short of the seam)
The journey got ~95% there: forest grind to L12, Pewter, **beat Brock
(badge 0x01)**, onto Route 3. It then reached Route 3's *final* trainer —
the Lass at (33,10), Jigglypuff L14 — the last one before the north-edge
seam to Route 4 (59,0) — and the battle **hung to the 60000-frame cap with
both mons alive** (our L17 water-type lead 37/52; Jigglypuff 38/58). Player
ended at (33,8) on map 0x0e, in front of her.

A `ZBAT=1` trace (`/tmp/cascade_zbat.log`) shows the exact mechanism:
- Our lead's top move is **WATER GUN** (power 40 + STAB); `StatAwareMove`
picks it and it lands (enemy 58→38).
- Jigglypuff replies **DISABLE** → "WARTORTLE's WATER GUN was disabled!".
- Next turn `StatAwareMove` *still* picks WATER GUN — it has no notion of a
disabled move, and `Usable()` filters only 0-PP — so the ROM bounces
"The move is disabled!" back to the menu. Repeat forever to the cap.
This is a **deterministic policy defect given the enemy's move choice**, not
RNG: whenever an enemy DISABLEs the policy's top-ranked move, `Battle`
stalls. It fires on some enemy-RNG cycles (S8-4 crossed Route 3 and won all
six — that cycle Jigglypuff never disabled our best move), so the journey is
**flaky, not impossible**.

### The defect (named and handed back, not fixed)
`skill.StatAwareMove` / `skill.Battle` do not model a move disabled by the
enemy's DISABLE. Fix is its own task: teach `Usable()`/the policy about the
disabled-move status, or have `Battle` detect the "The move is disabled!"
bounce and fall back to the next-best usable move. This is **not on S9-10's
surface** (my surface is the Cerulean gym / badge bit), so per the working
rules I name it and hand it back rather than adopt or fix it.

### Q4 in full: what did the generalised Gym assume that is Pewter-only?
From code inspection (no live win to confirm):
- **Data-driven, correctly generalised (not Pewter-specific):** the badge bit
  (`BadgeCascade` bit 1 vs `BadgeBoulder` bit 0), the leader tile (Misty
  (4,2) vs Brock (4,1)), the place name ("cerulean gym" → (4,3)), and the
  postcondition `Has(g.Badge)` all come from the `gyms` table. A Cerulean win
  checks bit 1, not bit 0 — the pre-generalisation bug is gone.
- **Pewter-calibrated constants (unverified for Cerulean):**
  `gymBattleWaitBudget=10000` (leader intro boxes) and
  `gymPostBattleBudget=3000` (post-victory boxes). Misty's post-battle script
  (`scripts/CeruleanGym.asm`) is structurally identical to Brock's — badge-info
  box → `GiveItem` TM11 → "received TM" box → **set `BIT_CASCADEBADGE`** →
  reset — same box count and the badge-written-before-reset ordering, so both
  budgets should fit. Read from the decomp, not measured.
- **The approach:** Cerulean's COOLTRAINER_F (2,3) faces right along row 3 and
  the SWIMMER (8,7) faces left across the walk column — `Travel` is expected to
  fight through both. Unmeasured (not reached).
- **Notable:** Misty's team (Staryu L18 + Starmie L21) knows only
  Tackle/Water Gun — **no DISABLE**. So the Route 3 blocker would *not* recur
  inside the actual Cerulean fight; had the journey reached the gym, the fight
  itself was not exposed to the defect that stopped the run.

### Tests
New `skill.TestGymCascadeBadge` (`-short`-skipped), alongside
`TestGymBoulderBadge`: post_errand → forest grind L12 → Pewter/Brock (hard
prereq) → Route 3 → Route 4 east-half grass grind to L24 → Cerulean gym →
`Gym(Misty)` → assert **raw** `wObtainedBadges` bit 1. It is currently
**blocked by the DISABLE defect on Route 3** (flaky in full mode) and should
join a full journey pass only once that is fixed; under `-short` (the per-task
gate) it skips, like `TestGymBoulderBadge`.

### Verification
- `go build ./...`, `go vet ./skill/`: clean.
- `go test ./skill -short`: ok (249s).
- Full run (no `-short`): reached Route 3's final trainer, hung on DISABLE
  (above). Failure state at `skill/failure/TestGymCascadeBadge.state`; trace
  at `/tmp/cascade_zbat.log`. Scratch `skill/zz_*` measurement files deleted.

### For the next task
- The DISABLE defect is the gating item for every Cerulean-and-beyond journey:
  Route 3's Lass, and any later trainer whose mon knows DISABLE. Fixing it
  unblocks this test and `TestCeruleanJourney`.
- S8-7's premise that "the journey to Cerulean works" was not quite right:
  S8-7 never reached Cerulean (it stopped at the forest on the Youngster
  stalemate). The Route 3 → Route 4 seam is not the blocker — the run got to
  the last Route 3 trainer without a geometry problem; the real gate is the
  DISABLE policy defect.


## S9-11: what does the menu cost now? (measurement)

Measurement task; **Offer was NOT changed** (verified by diff). Scratch
harness `agent/zz_menu_test.go` deleted. Method: load each fixture (and drive
one short journey for the maps without fixtures), Observe, Offer, count.
Knowledge = visited maps only, first visit, Talked empty, so these sizes are a
FLOOR — dialogue place names would only add goto entries. Pallet/Route 1/
Viridian City rows are pre-errand states; everything else is post-errand.

### Size table (measured 2026-08-30, Offer at 0c6c9b4+KindUseItem)

```
map                        total  per-kind
Pallet Town                    6   goto=3 talk=2 errand=1
Route 1                        6   goto=2 talk=2 errand=1 train=1
Viridian City                 13   goto=6 talk=6 errand=1
Viridian Mart                  7   goto=3 talk=3 buy=1
Viridian Pokemon Center        9   goto=4 talk=4 heal=1
Route 2                        7   goto=6 train=1
Viridian Forest               12   goto=6 talk=2 train=1 pickup=3
Viridian Forest (with balls)  17   goto=6 talk=2 train=1 catch=5 pickup=3
Pewter City                   15   goto=10 talk=5
Pewter Gym                    11   goto=8 talk=1 heal=1 gym=1
```

Where the entries come from: towns are expensive because of **talk** (one per
person on the map: 6 in Viridian, 5 in Pewter) and **goto** (one per place name
on a known map — Pewter's 10 come from 8 visited maps). The forest is
expensive because of **catch** (one per wild-table species: WEEDLE KAKUNA
METAPOD CATERPIE PIKACHU = 5) plus 3 field-item pickups. The no-balls chain
measured the forest at 12; a hunting run carries balls, so 17 is the real town/
forest ceiling so far.

### Latency and prompt tokens (qwen3.5-4b @ :8000, one ask each, two runs)

```
offered   light-load run            heavy-load run           prompt tokens
6         9.1 / 4.2 / 4.2 s         30 / 20 / 20.5 s         498-745
13-14     10.6 / 1.4; 10.9/1.8/1.7  44 / 29 / 27             642-827
15        11.0 / 1.5; 1.6 / 1.5     -                        657
18        -                         14.6 / 2.2               892
```

The 2026-08-29 baseline (5-7 -> 6-7s, 13-15 -> 21-23s) does NOT reproduce:
within each run, n=6 and n=15-18 cost the same, and server load (1.5s-44s
across the two runs) dominates over any size effect. Prompt tokens grow about
+30 per entry: 12 extra entries buy ~+390 prompt tokens.

**The real round cost is not the menu.** Every reply in both runs was rejected
for a superfluous argument (the S6-12 #3 defect: "level argument N does not
apply to go to route 1"), and `planWithRetries` re-asks up to MaxReplyRetries=3
times — so a round whose model keeps mis-arguing costs up to 3x one call, at
ANY menu size.

### Recommendation: do nothing to Offer

Sizes are 6-17; no measurable latency penalty at 18 vs 6 on today's server;
the prompt growth is small and bounded by the maps a run has actually visited.
Capping catch (5 -> 1 WithArgs entry, S9-1's path) would save ~130 tokens on
one map for no measured latency and would give back the per-species
specificity 5f379dd deliberately added. The next task that cuts round latency
is the superfluous-argument rejection storm (prompt/WithArgs validation), which
multiplies every call by up to 3 regardless of menu size.

## S9-12: the milestone — how far does a long goal actually get (measurement)

### Baseline drift: merge check (done before running anything)
The plan said main was at cc40b7f. It is not: 17edf44 (StatAwareMove now
scores power x type effectiveness x STAB from the ROM's TypeEffects chart,
not raw Power — changes the outcome of every battle) and d93ebc9
(Result.Err set ONLY when a failure stops the run) both landed on main
after the plan was written. **Merge check: no merge was needed.** HEAD
(b732082) already contains both commits via merge 4cb3233 made during the
S9-6 task; `git merge-base --is-ancestor` confirms 17edf44 and d93ebc9 are
ancestors of HEAD, and main's tip (835a010) is itself an ancestor of HEAD.
**Move policy used by this run: the NEW TypeEffects-based StatAwareMove
(17edf44), identical to what main fights with.** Result.Err semantics are
the d93ebc9 ones: Stop carries the reason, Err is nil on a recovered run.

### The harness change (the one allowed)

`cmd/badgerun` only, no agent code, no verbs, no prompt text (verified by
diff): `-badge` flag (default "Boulder") so a run chasing Cascade does not
auto-stop the moment Pewter is earned; `final.state` written to the run dir
after every run (the bounded checkpoint ring lags the last objective and
cannot stand in for where the run died); `badgePlanner.NextFeedback`
forwarding rejections. The third item is why the first attempt exists:
without it the wrapper failed the FeedbackPlanner type assertion, so
planWithRetries could not re-ask and ONE superfluous-argument reply stopped
the run on round 1 ask 1 (seed 7, pre-fix binary) — fixed and re-run, which
is the "fix that specific thing" case, recorded here. Scratch
`skill/zz_s912_final_test.go` (RAM read of final.state) deleted after use.

### The runs

Two runs, goal "Earn the Cascade Badge.", `-badge Cascade`, starter squirtle
(harness-controlled), seeds 7 and 11, max-rounds 256 (4x the badge run's
64), max-frames 8h emulated. qwen3.5-4b @ 192.168.50.204:8000, temperature
0. Both runs died on round 7, on the same map, on the same objective —
deterministic, not RNG: run 2 had a different encounter history (5 wild
battles vs 2) yet converged onto the identical wall, and with temperature 0
the model's reply is a function of the menu, which is the same menu at that
point for any seed. Further seeds are provably redundant; two was the
"more than once" the task asked for.

### Answer 1 — HOW FAR (read from RAM, not log prose)

`final.state` loaded into a fresh emulator and decoded
(`state.Read`/`DecodeParty`/`DecodeProgress`):

```
run 1 (seed 7):  map 0x29 VIRIDIAN_POKECENTER (3,3), controllable, no battle
                 badges [] (wObtainedBadges raw 0x00)
                 party[0] SQUIRTLE (species 177) L6 hp 21/21, money 3175
run 2 (seed 11): map 0x29 VIRIDIAN_POKECENTER (3,3), controllable, no battle
                 badges [] (raw 0x00)
                 party[0] SQUIRTLE L7 hp 25/25, money 3175
```

Map sequence from the round lines: 0x28 Oak's lab -> 0x00 Pallet -> 0x0c
Route 1 -> 0x01 Viridian City -> 0x29 Viridian Pokemon Center. It never
reached Viridian Forest, Route 2 or Pewter. The furthest point of the whole
slice so far was S9-10's deterministic journey (Route 3's last trainer);
the LLM planner has now been measured at its own furthest point: one town
past Route 1.

### Answer 2 — WHAT STOPPED IT

Stop reason: **error** (StopError), round 7, in both runs. The model kept
attaching a superfluous argument to the offered objective "talk at (3,1)" —
`"flee": true` in run 1, `"level": 7` then `"flee": true` in run 2 — and
repeated the IDENTICAL invalid reply through all three asks;
planWithRetries quoted the rejection back each time and the model ignored
it. MaxReplyRetries=3 exhausted -> the run stops. Result.Err carries that
error (d93ebc9 semantics: this failure actually stopped the run, so Err is
set — it would be nil on a recovered run). It is NOT the budget (7 of 256
rounds, ~16-22k of 7.5M frames) and NOT a missing verb: talk was offered
and is a working verb; the gate is reply-shape validation meeting a model
that fills every schema field regardless of kind. This is the S9-11
rejection storm (S6-12 #3, "level argument N does not apply to go to route
1"), which predates slice 9's intent work — the storm was 100% of S9-11's
measured replies.

### Answer 3 — DID THE INTENT HOLD (the number the slice is about)

**The model did NOT ignore the field.** It wrote an intent on every round,
from round 1 (ask 2) through 7, and both runs' timelines are byte-identical:

```
round 1: "Deliver Oak's parcel to progress toward the Cascade Badge."
round 2: "Return to Pallet Town to access the route leading to the Cascade Badge."
round 3: "Head to Route 1 to begin the journey toward the Cascade Badge."
round 4: "Head to Viridian City to progress toward the Cascade Badge."
rounds 5-7: "Heal the party at the Viridian Pokemon Center to recover HP
             before continuing toward the Cascade Badge."
```

It changed every round while the next step changed (4 distinct sentences in
5 rounds) and held unchanged once the step stopped changing. Longest
survival: the heal intent, carried through 3 observations with IntentAge
reaching 1 — the run died before it could age further. Every sentence is
suffixed "...toward the Cascade Badge": the goal statement is bound into
the intent, but the intent itself is STEP-level, not goal-level.

The milestone question — can a run hold an objective LONGER than it can
remember — is therefore **unanswerable at this point**: the mechanism works
as designed (S9-4's round trip holds live), but no run can outlive round 7
to show a long hold. That is the finding slice 10 must size against: fix
the rejection storm first, or the intent question stays open forever.

### Answer 4 — DID THE FLEE ARGUMENT GET USED (S9-1)

Yes, on every travel leg: rounds 2-5 were all "go to X, fleeing wild
battles". Run 1: 2 wild encounters, all fled. Run 2: 5 wild encounters, all
fled. Blackouts: 0 in both runs; the party ended at FULL HP (run 2's lead
even reached L7 from the fled encounters). Against a comparable earlier
run: there is no comparable earlier LLM run — no long LLM-driven run has
ever completed, so the fights-avoided-vs-blackouts comparison has no prior
LLM baseline. The three-blackouts-per-journey baseline is a deterministic-
journey number from the grind phase, which this run never reached; the
comparison is vacuous beyond "flee worked exactly as designed on the legs
it was used on".

### Answer 5 — WHAT DID IT HEAR (S9-6)

Nothing. Requirements was `[]` in all 11 observations of both runs. The only
dialogue the runs had was the town-map woman ("I'll borrow a TOWN MAP from
my sis!") and the nurse ("Shall we heal your POKéMON?") — neither carries
the "you can pass here only if you have X" shape HeardRequirement harvests.
No requirement sentence reached the observation, so no claim about choice
change after one is supportable from these logs. The proof case (a guard
stating a requirement) is past Route 1's reach; it was never in these runs.

### Answer 6 — DID TRAIN RETREAT (S9-9)

No train objective was ever offered or executed: the run stopped before the
first grass grind, so there were 0 retreats and 0 blackouts. The comparison
against the three-blackouts-per-journey baseline is vacuous for the same
reason as answer 4 — the run never entered the phase where that baseline
applies.

### Cost (part of the result)

```
              run 1 (seed 7)     run 2 (seed 11)
wall clock    156 s              177 s
rounds        7 attempted, 6 ok  7 attempted, 6 ok
llm calls     11 (4 re-asks)     11 (4 re-asks)
offered/ask   5-13 (avg 8.6)     5-13 (avg 8.6)
prompt tokens 612-793, avg 716   612-793, avg 716
completion    24-49,  avg 35     24-49,  avg 36
latency/call  2.7-19.7 s, avg 12.7 s   2.7-19.7 s, avg 14.1 s
total tokens  7874p / 390c       7875p / 401c
```

Against S9-11's table (not re-derived): prompt tokens at n=5-13 offered sit
inside its 498-827 band and the ~+30/entry rule holds (n=13 -> 793).
Latency is between S9-11's light (4-11s) and heavy (20-44s) samples —
server load dominates, as it concluded. The storm is the dominant cost:
each rejection costs a full extra call (~13-20s), and the round that killed
the run cost 3 calls.

### What slice 10 gets sized from

1. The milestone is gated by the rejection storm, not by game distance.
   Until superfluous-argument replies stop killing runs (S9-11's
   recommendation: prompt/WithArgs validation, up to 3x cost per round),
   no long-goal measurement can get past ~round 7, and the intent-hold
   question (answer 3) stays open.
2. Intent is used actively and is step-level; a long hold has never been
   observed because no run lives long enough.
3. Flee and the requirement harvester are wired but unexercised at depth:
   flee worked on every leg it got, and no requirement sentence was ever in
   range of these runs.

### Handoff for the next task (S9-13 / slice 10)

What changed: `cmd/badgerun` only — `-badge` flag (default Boulder), a
`final.state` dump per run, and `badgePlanner.NextFeedback` forwarding so
rejection retries work in badgerun (without it one bad reply killed the run).
No agent code, no verbs, no prompt text. Scratch zz test deleted.

What the next task must know:
- The milestone (S9-12) is DONE and its answer is above: two runs, both dead
  on round 7 at Viridian Pokemon Center, zero badges, gated by the S9-11
  rejection storm (superfluous flee/level args on talk, identical reply
  repeated through all 3 retries). Further seeds are redundant (temp 0).
- Slice 10's first job is the reply-shape gate (prompt/WithArgs validation).
  Until then no long-goal measurement can pass ~round 7 and the intent-hold
  question stays open. Intent IS used actively (step-level, goal-suffixed).
- Run artifacts kept at /tmp/s912-run{1,2}/ (run.log, prompts.txt,
  final.state) if anyone wants to re-derive a number; they are not in the
  repo and may be gone.
- To re-run: `badgerun -goal "Earn the Cascade Badge." -badge Cascade
  -starter squirtle -n 1 -seeds <N> -max-rounds 256 -max-frames 20736000`,
  with POKEMON_RED_ROM and llm_token set (server: qwen3.5-4b only, no bigger
  model available for ablation A).

## S10-1: the reply-shape gate — flee moves out of the reply and into the menu (farm hotfix)

### The bug, measured

The farm (pokemon.labstack.cc) was dying on this. Dashboard, 79 runs:
36 done/error, of which ~30 are one error, in four shapes:

```
agent: llm planner: agent: flee argument true does not apply to talk at (5,24)
agent: llm planner: agent: flee argument true does not apply to deliver oak's parcel
agent: llm planner: agent: flee argument true does not apply to talk at (3,1)
agent: llm planner: agent: flee argument true does not apply to talk at (8,3)
```

Reproduced locally with `make run-llm` (seed 0): round 1, the model replied
`{"choice": 1, "flee": true, ...}` for "take the charmander starter", was
rejected, and repeated the IDENTICAL reply through all 3 asks (temp 0 — the
feedback text changes the prompt but not this model's answer), so the run
stopped with StopError after 0 completed rounds. This is S9-12's rejection
storm, confirmed on the live farm, and slice 10's first job per that
handoff.

Root cause: `flee` was a CONDITIONAL argument in one flat reply schema —
legal only on go-to and heal-with-place, but present in the schema (and the
system prompt) for every choice. A small model given the field emits it on
every reply it can; WithArgs rejects the misplaced flag; at temperature 0
the rejection feedback does not change the answer; MaxReplyRetries runs out;
the run dies. The other arguments (level/species/item/quantity) are the same
class but were not observed misfiring in production: they override values
the menu already shows, while flee was a hidden boolean visible nowhere in
the offered list.

### The fix

The choice is made in the MENU, where the model demonstrably is reliable
(picking an index), not in the reply, where it is not:

- `agent/offer.go`: every journey — each go-to, and the travelling heal —
  is offered twice, plain then its fleeing variant, adjacent. The variant's
  String() already existed ("go to X, fleeing wild battles", S9-1).
- `agent/llm.go`: `"flee"` removed from choiceSchema. The constrained
  decoder (llama.cpp json_schema) forbids whatever the schema omits, so a
  conforming server CANNOT emit the field — the rejection is prevented
  rather than detected. System prompt reworded: travelling objectives come
  in two variants; pick the one you want by number.
- `choiceReply` keeps the `Flee` field and WithArgs still rejects a
  misplaced one: a server that ignores response_format and emits "flee" in
  prose-JSON is rejected as before, never silently dropped.

This is stronger than S9-12's proposed "prompt/WithArgs validation":
validation after the fact is what failed (3 identical rejections), so the
field is gone from the grammar instead.

### Verification

- `go test ./...` green. TestOfferTable's exact menus updated; new
  TestOfferJourneyVariants pins plain-then-variant adjacency for every
  journey; TestLLMPlannerSendsSchema now pins that the request schema does
  NOT contain `"flee"` (regression gate on this exact bug).
- `make run-llm` against the live server, before: dead at round 1 (3
  identical rejections). After: rounds 1-4 clean — starter taken, parcel
  delivered, two journeys walked, zero re-asks, replies now
  `{"choice": N, "intent": ...}` only.

### Cost (part of the result)

The menu grows by one entry per journey. Per S9-11's ~+30 tokens/entry and
~1.4s/entry rules, a run with 3-4 journeys pays ~+90-120 prompt tokens and
~4-6s per call — minutes over a 32-round run, against a farm where the same
runs died at round 1-7. The model handled the larger menus (9-14 offered)
without a single index miss in the verification run.

### Left as-is (measured, not fixed)

- 6 of 79 runs ended "no heartbeat for 31-33s": the runner's heartbeat
  goroutine ticks every second, so this is the reaper doing its job on dead
  or wedged runner tasks (Swarm restart churn), not a planner bug.
- The remaining error details ("text box interrupted movement", "connection
  edge ... did not cross within 180 frames", "no path ...") are skill-level
  traversal outcomes — the game answering, per the standing rule. They are
  candidates for slice 10's next measurement, not this fix.

## S10-2: talk through the grass — TalkAt's approach flees wild encounters instead of dying on them

### The bug, measured

`make run-llm`, post-S10-1, died at round 18 — two consecutive identical
failures of one objective, then the failure budget ran out:

```
round 18: talk at (5,24) -> map 0c at (12,22)
round 19: talk at (5,24) -> map 0c at (12,22)
run stopped: failed after 19 round(s)
  error: agent: talk at (5,24): skill: TalkAt: approach object 1 at (5,24):
         skill: Approach: walk beside (5,24) on map 0x000c:
         skill: battle interrupted movement
```

The same class was killing farm runs: three dashboard hits on `talk at
(15,13)` / `(14,13)` on map 0x000c (Route 1).

Root cause: TalkAt's approach was a raw `WalkPath`, which aborts on the first
wild battle by design ("callers decide what to do"). NPCs stand in tall grass
as often as in towns — the Route 1 old man at (5,24), the (15,13) pair — so
every approach that crosses grass died on the first encounter; the model
re-offered the same objective and it died again. The codebase had already
solved exactly this for ground items: `Pickup`'s `approachViaTravel` uses
Travel, with the comment "Approach aborts on the first wild battle by design
— a pickup objective there would fail on every retry". TalkAt was the outlier.

### The fix

- New shared planner `besideDestination` (skill/interact.go): picks the
  walkable tile orthogonally adjacent to the target that is closest
  (Manhattan) to the player; ok=false when already adjacent. `Pickup`'s
  `approachViaTravel` now uses it — behaviour and error strings unchanged.
- New `talkBeside`: walks to the beside-tile via **TravelFlee**, not a raw
  walk. Flee, not fight: the walk is logistics for the conversation, not an
  end in itself — a fight here is damage and level-up math the objective
  never asked for, and a loss blackouts the party into a town, so the NPC's
  map may no longer be the current one when it returns. Fleeing never loses
  (S8-4 measured first-attempt success in this ROM). A trainer battle — which
  the game refuses to let you flee — is fought with the policy; a blackout
  from that fallback comes back as ErrBlackedOut for the caller to decide on.
- `TalkAt` takes a `policy MovePolicy` (mirroring Pickup); the agent passes
  StatAwareMove.
- `Approach` deleted: zero callers after this change, and its contract (dies
  on a battle) is exactly the trap that killed these runs.

### Verification

- New `TestExecuteTalkCrossesTallGrass` (agent) with a new `route1` fixture
  (post-story, Place("route 1") center (5,14) on 0x0c): KindTalk at (5,24),
  whose approach crosses tall grass. Postconditions are both halves: the
  player must END UP on 0x0c orthogonally adjacent to (5,24) (the talk
  happened where it was offered, not in a town after a blackout), and the
  lead's HP must be unchanged (the approach fled; a fought encounter would
  have damaged it). Passes in ~11s.
- `go test -short ./...` green; `go test -count=1 ./agent` green.
- `make run-llm` live: **all 32 rounds completed** ("run stopped: budget",
  the normal ending) where the previous runs died at rounds 1–19. The model
  used the S10-1 flee variants twice ("go to route 1, fleeing wild battles",
  "go to viridian city, fleeing wild battles"), trained on Route 1's grass,
  healed at the Viridian center, and completed five talk objectives across
  Oak's lab and Viridian City. One self-recovered rejection in 32 rounds
  (below).

### Observation, not fixed (self-recovering)

Round 23: the model sent `{"choice": 11, "level": 9, "species":
"{...}"}` for train — cross-wiring the catch objective's `species` argument
onto train. WithArgs rejected it; the model corrected to `{"choice": 11}` on
ask 2 and the round completed. Same flat-schema shape as S10-1's flee, but
recoverable: unlike flee (emitted on every reply, temp 0, identical answer),
the species field appeared once and the feedback changed the answer. If it
starts burning retries, the fix is per-objective argument schemas, not
prompting.

### Left as-is (measured, not fixed)

- Two dashboard hits on `talk at (12,4): Approach: no path on map 0x0034
  from (10,4) beside (12,4)`: a genuinely unreachable beside-tile — a
  different class (geometry or door state), needs a saved state to measure.
- `TestGymCascadeBadge` full-mode flake is the documented DISABLE policy
  defect (named and handed back in S9-10): this session's failure reproduced
  the recorded battle state byte-for-byte — L17 lead 37/52 vs Jigglypuff
  38/58 at the 60000-frame cap, Route 3 (33,8). Not on this slice's surface.
