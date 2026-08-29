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
