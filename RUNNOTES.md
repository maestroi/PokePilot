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
