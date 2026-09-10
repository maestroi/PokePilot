# LLM World Model and Recovery Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Give the objective-level planner accurate live NPC blockers, named maps, structured and verified objective outcomes, and bounded recovery from ordinary dialogue interruptions without allowing movement code to answer choices.

**Architecture:** Keep the existing three-level boundary: `agent` chooses objectives, `skill` owns tactics, and movement/menu helpers own inputs. The ROM graph and tile grid remain the base world model; every path plan overlays a fresh sprite-RAM snapshot. `Execute` returns an `ObjectiveResult`, a normalization step turns raw skill failures into planner-facing outcomes, and `Run` continues only after a controllable-overworld state gate and terminal overrides pass.

**Tech Stack:** Go 1.26, SameBoy-backed emulator, vendored `pokered` decomp, standard `testing`, existing on-demand ROM fixtures.

---

## Execution prerequisite

The checkout used to write this plan has concurrent uncommitted work, including
planner/objective files, navigation files, command wiring, tests, probes, and
new documentation. Treat `git status --short` at execution time as the source
of truth. Do not reset, stash, stage, or absorb those files on the user's
behalf.

Before Task 1, preserve that work on its intended branch, then use `superpowers:using-git-worktrees` to create an isolated implementation worktree from the commit containing it. Confirm the baseline there:

```bash
git status --short
go test ./...
make test
```

Record any pre-existing failure and stop; do not hide it with implementation changes.

The commit steps below are checkpoints for an interactive execution. When run
under `agent-runner`, follow `AGENTS.md` instead: do not commit, and leave the
verified task diff uncommitted for the runner to collect.

## Task 1: Make re-plan exhaustion unambiguously terminal

**Files:**

- Modify: `skill/goto.go`
- Create: `skill/goto_replan_test.go`

### Step 1: Write the failing error-chain test

Add a package-internal test so it can exercise a small constructor without running nine real route attempts:

```go
func TestReplanExhaustedKeepsBothIdentities(t *testing.T) {
	last := fmt.Errorf("route 2 north edge: %w", ErrLegUnwalkable)
	err := newReplanExhaustedError(8, 0x0d, 8, 71,
		Destination{Map: 0x02, X: 14, Y: 8}, last)

	if !errors.Is(err, ErrReplanExhausted) {
		t.Fatalf("missing ErrReplanExhausted: %v", err)
	}
	if !errors.Is(err, ErrLegUnwalkable) {
		t.Fatalf("missing ErrLegUnwalkable cause: %v", err)
	}
}
```

### Step 2: Run it and confirm the missing API fails

```bash
go test ./skill -run TestReplanExhaustedKeepsBothIdentities -count=1
```

Expected: compile failure because the sentinel and constructor do not exist.

### Step 3: Add the sentinel and wrapping helper

In `skill/goto.go`:

```go
var ErrReplanExhausted = errors.New("skill: route re-plan budget exhausted")

func newReplanExhaustedError(max int, cur, x, y uint8, dest Destination, last error) error {
	return fmt.Errorf("%w: %d re-plans from map %02x at (%d,%d) toward map %02x at (%d,%d), last leg: %w",
		ErrReplanExhausted, max, cur, x, y, dest.Map, dest.X, dest.Y, last)
}
```

Replace the current exhaustion return with this helper. Keep the final `ErrLegUnwalkable` in the chain; later policy checks `ErrReplanExhausted` first.

### Step 4: Verify and commit

```bash
go test ./skill -run 'TestReplanExhaustedKeepsBothIdentities|TestWalkAroundGivesUpAfterMaxRetries' -count=1
go test ./skill -count=1
git add skill/goto.go skill/goto_replan_test.go
git commit -m "fix: type route replan exhaustion"
```

## Task 2: Decode live sprite tiles and anchor the coordinate convention

**Files:**

- Create: `red/state/sprite.go`
- Create: `red/state/sprite_test.go`
- Create: `skill/sprite_fixture_test.go`

### Step 1: Write synthetic slot-selection tests

Define the intended public shape:

```go
func TestDecodeSpritesExcludesPlayerAndInactiveSlots(t *testing.T) {
	var mem Mem

	// Slot 0: active player, never returned.
	mem[sym.SpritePlayerStateData1+0x00] = 1
	mem[sym.SpritePlayerStateData1+0x02] = 2
	mem[sym.SpriteStateData2+0x04] = 4 + 9
	mem[sym.SpriteStateData2+0x05] = 4 + 7

	// Slot 1: active and on-screen. Data2 picture scratch is deliberately 0.
	data1 := sym.SpritePlayerStateData1 + 0x10
	data2 := sym.SpriteStateData2 + 0x10
	mem[data1+0x00] = 11
	mem[data1+0x02] = 2
	mem[data2+0x04] = 4 + 3
	mem[data2+0x05] = 4 + 5

	// Slot 2: real object, but hidden/off-screen.
	data1 = sym.SpritePlayerStateData1 + 0x20
	data2 = sym.SpriteStateData2 + 0x20
	mem[data1+0x00] = 12
	mem[data1+0x02] = 0xff
	mem[data2+0x04] = 4 + 8
	mem[data2+0x05] = 4 + 8

	got := DecodeSprites(&mem)
	want := []SpriteState{{Slot: 1, X: 5, Y: 3, PictureID: 11}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodeSprites() = %+v, want %+v", got, want)
	}
}
```

Also test active slot 15 so a wrong upper bound is visible.

Run and require the missing-API failure:

```bash
go test ./red/state -run TestDecodeSprites -count=1
```

### Step 2: Implement the decoder

In `red/state/sprite.go`, keep layout constants local to the decoder:

```go
const (
	spriteStateStride      = uint16(0x10)
	spritePictureIDOffset  = uint16(0x00) // data1
	spriteImageIndexOffset = uint16(0x02) // data1; 0xff is hidden/off-screen
	spriteMapYOffset       = uint16(0x04) // data2
	spriteMapXOffset       = uint16(0x05) // data2
	spriteSlots            = 16
	spriteCoordinateBias   = 4
)

type SpriteState struct {
	Slot      int
	X, Y      int
	PictureID uint8
}

func DecodeSprites(m *Mem) []SpriteState {
	out := make([]SpriteState, 0, spriteSlots-1)
	for slot := 1; slot < spriteSlots; slot++ {
		data1 := sym.SpritePlayerStateData1 + uint16(slot)*spriteStateStride
		picture := m.U8(data1 + spritePictureIDOffset)
		if picture == 0 || m.U8(data1+spriteImageIndexOffset) == 0xff {
			continue
		}
		data2 := sym.SpriteStateData2 + uint16(slot)*spriteStateStride
		out = append(out, SpriteState{
			Slot: slot,
			X: int(m.U8(data2+spriteMapXOffset)) - spriteCoordinateBias,
			Y: int(m.U8(data2+spriteMapYOffset)) - spriteCoordinateBias,
			PictureID: picture,
		})
	}
	return out
}
```

Do not add `wNumSprites`: unused data1 slots have picture ID zero. Do not use
`SPRITESTATEDATA2_PICTUREID` at `+0x0d`; map sprite loading zeroes that scratch
byte after tile patterns are loaded.

### Step 3: Add the independent real-ROM anchor

In `skill/sprite_fixture_test.go`, load `viridian_pokecenter`, assert map `0x29`, snapshot RAM, parse the same map, and compare decoded slot 1 (the stationary nurse) with `MapHeader.Objects[0]`:

```go
sprites := state.DecodeSprites(&mem)
nurse, ok := spriteAtSlot(sprites, 1)
if !ok {
	t.Fatalf("slot 1 nurse inactive; sprites=%+v", sprites)
}
want := header.Objects[0] // ParseMap already removes the ROM +4 bias.
if nurse.X != int(want.X) || nurse.Y != int(want.Y) {
	t.Fatalf("nurse RAM=(%d,%d), ROM=(%d,%d); sprites=%+v",
		nurse.X, nurse.Y, want.X, want.Y, sprites)
}
```

Decode once, validate `len(header.Objects) > 0` before indexing, and keep `spriteAtSlot` test-local. This comparison is required because a synthetic test cannot catch a shared wrong stride, offset, or `-4` assumption.

### Step 4: Verify and commit

```bash
go test ./red/state -run TestDecodeSprites -count=1
go test ./skill -run TestDecodedNurseTileMatchesROMObject -count=1
git add red/state/sprite.go red/state/sprite_test.go skill/sprite_fixture_test.go
git commit -m "feat: decode live sprite positions"
```

The fixture test passes with `POKEMON_RED_ROM` and otherwise skips through the fixture helper.

## Task 3: Replace inferred bans with fresh live blockers

**Files:**

- Create: `skill/blockers.go`
- Modify: `skill/move.go`
- Modify: `skill/goto.go`
- Modify: `skill/warp.go`
- Modify: `skill/story.go`
- Modify: `skill/walkaround_test.go`

### Step 1: Replace ban tests with race and forgetting tests

Keep `TestWalkAroundGivesUpAfterMaxRetries` and `TestWalkAroundDoesNotRetryOtherFailures`. Replace tests that assert guessed bans with two independent regressions:

```go
func TestWalkAroundRereadsBlockersAfterCollision(t *testing.T) {
	// Read 1: sprite blocks (14,13). First walk collides.
	// Read 2: sprite moved to (15,13). Second walk succeeds.
	// Assert both snapshots reached plan in order.
}

func TestWalkAroundForgetsVacatedSpriteTile(t *testing.T) {
	// Read 1 contains (14,13); read 2 is empty.
	// Assert plan 2 receives no remembered copy of (14,13).
}
```

The race test is collide-then-re-read. The forgetting test proves the absence of a cache even without a second collision.

Run:

```bash
go test ./skill -run 'TestWalkAround|TestSpriteBlockers' -count=1
```

Expected: failures because `walkAround` still owns `hit` and `blocked` maps.

### Step 2: Add the live overlay

In `skill/blockers.go`:

```go
func spriteBlockers(m *emu.Emu) map[[2]int]bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	blocked := make(map[[2]int]bool)
	for _, s := range state.DecodeSprites(&mem) {
		blocked[[2]int{s.X, s.Y}] = true
	}
	return blocked
}

func mergeBlockers(live, fixed map[[2]int]bool) map[[2]int]bool {
	out := make(map[[2]int]bool, len(live)+len(fixed))
	for p := range live { out[p] = true }
	for p := range fixed { out[p] = true }
	return out
}
```

Because the data1 image-index filter excludes off-screen sprites, this overlay
is deliberately screen-local. A distant sprite becomes a blocker only when the
engine makes it visible. Also document that `TryWalking` writes a moving NPC's
destination MAPY/MAPX before its 16-frame animation finishes, so the sprite can
visually straddle two tiles. Both cases are races handled by the bounded retry,
not reasons to add a cache or a second inferred tile.

Change `walkAround` to accept `readBlocked func() map[[2]int]bool`. At the top of every attempt, call it exactly once and pass that fresh map to `plan`.

- On a plan error with live blockers present, wait and retry until `maxWalkRetries`; with no live blockers, return the static/path error.
- On `*ErrBlocked`, wait and retry from a new snapshot.
- On any other walk error, return immediately.
- Delete all collision-derived `hit`/`blocked` memory.

Keep `maxWalkRetries` and `npcWaitFrames`.

Put this invariant above `walkAround`:

> Live sprite positions are ephemeral observations, never learned world geometry. Every plan rebuilds blockers from current RAM. A collision retries from new RAM; no blocker cache exists, so there is nothing to expire or forget.

### Step 3: Wire every path planner

Update each `walkAround` call in `skill/goto.go` and `skill/warp.go` to pass a fresh `spriteBlockers(m)` reader.

Refactor `walkLab` in `skill/story.go` onto the same helper. Preserve truly static exclusions such as ball tiles by merging them into every fresh snapshot. NPCs come from RAM; no collision may mutate the fixed set.

Put this exact invariant above `WalkPath` now and above the recovery loop in Task 6:

> Movement never advances dialogue. After dialogue has interrupted movement, the recovery layer may press A only while ordinary text is active. It never answers a choice.

### Step 4: Verify and commit

```bash
go test ./skill -run 'TestWalkAround|TestSpriteBlockers|TestGoTo|TestTraverse' -count=1
go test ./skill ./world -count=1
git add skill/blockers.go skill/move.go skill/goto.go skill/warp.go skill/story.go skill/walkaround_test.go
git commit -m "feat: plan around live sprite blockers"
```

## Task 4: Populate map names from a checked pure table

**Files:**

- Create: `red/state/map_names.go`
- Create: `red/state/map_names_test.go`
- Modify: `agent/observe.go`
- Modify: `agent/observe_test.go`

### Step 1: Pin the mapping without a ROM

```go
func TestMapName(t *testing.T) {
	cases := []struct{ id uint8; want string }{
		{0x00, "PALLET_TOWN"},
		{0x28, "OAKS_LAB"},
		{0x02, "PEWTER_CITY"},
		{0xff, ""},
	}
	for _, tc := range cases {
		if got := MapName(tc.id); got != tc.want {
			t.Errorf("MapName(%#02x) = %q, want %q", tc.id, got, tc.want)
		}
	}
}
```

Change `TestObserveFreshBoot` to expect `REDS_HOUSE_2F` for map `0x26`, and populate `MapName` in the JSON round-trip fixture.

Run and require failure:

```bash
go test ./red/state ./agent -run 'TestMapName|TestObserveFreshBoot|TestObserveJSONRoundTrip' -count=1
```

### Step 2: Add the plain table and a decomp-parity test

In `red/state/map_names.go`, add a `[256]string` literal keyed by the explicit
IDs in `pokered/constants/map_constants.asm`:

```go
var mapNames = [256]string{
	0x00: "PALLET_TOWN",
	0x01: "VIRIDIAN_CITY",
	// ...the remaining map_const entries...
	0x28: "OAKS_LAB",
}

func MapName(id uint8) string { return mapNames[id] }
```

In `red/state/map_names_test.go`, follow the existing
`event_constants_test.go` approach: parse the vendored
`../../pokered/constants/map_constants.asm`, extract each
`map_const NAME, ... ; $ID` line, and assert the complete Go table agrees.
Fail on duplicate IDs, duplicate names, malformed `map_const` lines, a missing
Go entry, or a Go entry not present in the decomp. The decomp is fixed inventory;
do not add a generator, generated file, `go:generate`, or drift build step.

Wire `Observe`:

```go
MapName: state.MapName(gs.Player.MapID),
```

No fixture or runtime ROM parsing belongs in this lookup.

### Step 3: Verify and commit

```bash
gofmt -w red/state/map_names.go red/state/map_names_test.go agent/observe.go agent/observe_test.go
go test ./red/state ./agent -count=1
git add red/state/map_names.go red/state/map_names_test.go agent/observe.go agent/observe_test.go
git commit -m "feat: expose stable map names"
```

## Task 5: Return structured, postcondition-checked objective results

**Files:**

- Create: `agent/objective_result.go`
- Modify: `agent/objective.go`
- Modify: `agent/objective_test.go`
- Modify: `agent/observe_test.go`
- Modify: `agent/run.go`
- Modify: `agent/run_test.go`

### Step 1: Write the result-contract tests

Update callers for the new signature, then assert a successful `KindGoTo` result:

```go
got, err := agent.Execute(e, e.ROM(), o)
if err != nil { t.Fatal(err) }
if got.Objective != o { ... }
if got.Outcome != agent.OutcomeCompleted { ... }
if got.Final.Map != dest.Map || got.Final.X != dest.X || got.Final.Y != dest.Y { ... }
if got.Travel == nil { t.Fatal("Travel = nil") }
```

Extend a Pallet-to-Viridian test to prove `TravelResult.Battles`, `Replans`,
and `BlackedOut` survive without flattening. Add result tests for the complete
`TrainResult` and for Gym win/loss. A Gym loss and a training shortfall are
structured non-completion outcomes the planner can reason from, not successful
completion hidden behind a log line. Do not capture stdout; structured data is
the behavior that matters.

Add pure postcondition cases:

- exact `KindGoTo` destination and controllable -> `OutcomeCompleted`;
- wrong map/tile after claimed success -> `OutcomePostconditionFailed`;
- still transient/uncontrollable after the settle budget ->
  `OutcomePostconditionUnavailable`.

Run:

```bash
go test ./agent -run 'TestExecute|TestObjectivePostcondition' -count=1
```

Expected: signature and missing-type failures.

### Step 2: Define the result contract

In `agent/objective_result.go`:

```go
type Outcome string

const (
	OutcomeCompleted                Outcome = "completed"
	OutcomeBlocked                  Outcome = "blocked"
	OutcomeChoiceRequired           Outcome = "choice_required"
	OutcomeStabilizationFailed      Outcome = "stabilization_failed"
	OutcomeOwnershipFailure         Outcome = "ownership_failure"
	OutcomeControllerUncertain      Outcome = "controller_uncertain"
	OutcomePostconditionFailed      Outcome = "postcondition_failed"
	OutcomePostconditionUnavailable Outcome = "postcondition_unavailable"
	OutcomeUnknownFailure           Outcome = "unknown_failure"
)

type ObjectiveResult struct {
	Objective  Objective
	Outcome    Outcome
	Summary    string
	Final      Observation
	Travel     *skill.TravelResult
	Train      *skill.TrainResult
	GymOutcome *state.BattleResult
}
```

Keep the low-level `error` as `Execute`'s second return value; do not put an `error` interface in planner JSON.
The outcome code is also the only postcondition status; do not add a second
tri-state field that can disagree with it.

### Step 3: Change Execute, without recovery yet

Change the signature:

```go
func Execute(m *emu.Emu, romData []byte, o Objective) (ObjectiveResult, error)
```

Initialize `Objective` immediately. Preserve complete `TravelResult`,
`TrainResult`, and Gym battle results. Remove all four objective-level
`fmt.Printf` calls in `agent/objective.go`: travel blackout, training blackout,
Gym win, and Gym loss. Put those facts in `Summary` and the typed fields.
Snapshot `Final` on every return path.

Map explicit skill results semantically: Gym win and `TrainResult.Reached` can
complete their objectives; Gym loss, training blackout, or a bounded training
shortfall return `OutcomeBlocked` rather than silently returning
`OutcomeCompleted`.

After reported success, use a named bounded settle helper only when required, then evaluate the semantic postcondition. `KindGoTo` requires exact destination plus controllable overworld. Existing positive assertions satisfy `KindStarter` and `KindTalk`; record them rather than replaying those contracts.

A false claimed-success postcondition returns `OutcomePostconditionFailed` and a typed terminal error. A check still impossible after its settle budget returns `OutcomePostconditionUnavailable` and a different typed error.

### Step 4: Adapt Run mechanically

Change `Run` only enough to compile and keep today's stop-on-any-error behavior.
Add `Outcomes []ObjectiveResult` to the whole-run `Result` now, initialize it,
and append to `Completed` only on `OutcomeCompleted`. Task 8 will begin retaining
all normalized results in `Outcomes`. Do not update the load-bearing `Run`
comment until Task 8 changes behavior.

### Step 5: Verify and commit

```bash
go test ./agent -run 'TestExecute|TestObjectivePostcondition|TestRun' -count=1
go test ./agent ./skill -count=1
git add agent/objective_result.go agent/objective.go agent/objective_test.go agent/observe_test.go agent/run.go agent/run_test.go
git commit -m "feat: return structured objective results"
```

## Task 6: Measure and implement guarded dialogue stabilization

**Files:**

- Create: `skill/dialogue_recovery.go`
- Create: `skill/dialogue_recovery_test.go`
- Create: `skill/dialogue_recovery_real_test.go`
- Create: `red/state/choice.go`
- Create: `red/state/choice_test.go`
- Modify: `skill/story.go`
- Modify: `skill/move.go`

### Step 1: Measure the tilemap before choosing the predicate

Create a temporary `skill/zz_dialogue_choice_probe_test.go` and inspect two real
states without implementing a predicate yet:

1. A live two-option menu, using a fixture-backed known yes/no flow.
2. The Viridian old-man ordinary dialogue: load `post_starter`, `Travel` to
   Viridian, use a test-local named `Destination` for the safe south approach,
   trigger the script, and capture each pre-A tilemap.

Log `FontLoaded`, `TextBoxID`, cursor/max, `JoyIgnore`, `ScreenText`, raw rows
containing the cursor/options, and the final deposit tile. Dump the rows as raw
tile IDs from `m.Slice(sym.TileMap, sym.TileMapLen)` in 20-wide lines, not
through `ScreenText`: `DecodeTiles` has no `textChars` entry for the `▶` cursor
(charmap `$ed`), so it renders as a space, and `strings.Fields` then collapses
away the row boundaries the predicate needs. Do not add the gate
approach to production `places`: `PlaceNames()` would offer a measurement
coordinate to the LLM.

Run verbose output to a file, inspect the small relevant excerpts, then delete
the scratch test before the task commit:

```bash
go test ./skill -run TestDialogueChoiceProbe -count=1 -v > /tmp/pokepilot-dialogue-choice.log 2>&1
rg -n 'choice|ordinary|TextBoxID|cursor|deposit' /tmp/pokepilot-dialogue-choice.log
```

Do not proceed until the measurement identifies a positive live tilemap shape
and establishes the old-man budget and safe deposit. If it does not, revise the
design rather than falling back to `TextBoxID`.

### Step 2: Write the choice and recovery tests from the measurement

In `red/state/choice_test.go`, test a decoder that returns non-nil only for the
measured live shape: `FontLoaded != 0`, a filled menu cursor tile, and two known
option strings in the corresponding tilemap rows. `DecodeTwoOptionMenu` reads
raw tile IDs out of `sym.TileMap` as 20-wide rows and compares the cursor tile
(`0xed`) by value; it must not go through `ScreenText` or `DecodeTiles`, which
erase both the cursor glyph and the row layout. `drawOrdinaryTextForTest` and
its live-menu counterpart therefore write tile IDs into `mem` at `sym.TileMap`
offsets. Include this adversarial row:

```go
func TestDecodeTwoOptionMenuRejectsStaleIDDuringOrdinaryText(t *testing.T) {
	var mem Mem
	mem[sym.FontLoaded] = 1
	mem[sym.TextBoxID] = 0x14       // stale TWO_OPTION_MENU
	mem[sym.CurrentMenuItem] = 0   // stale plausible cursor
	mem[sym.MaxMenuItem] = 1       // stale plausible shape
	drawOrdinaryTextForTest(&mem, "YES, I KNOW")

	if got := DecodeTwoOptionMenu(&mem); got != nil {
		t.Fatalf("ordinary text decoded as choice: %+v", got)
	}
}
```

Also test both cursor positions on a real two-option tilemap shape, a missing
cursor, incomplete/misaligned option strings, and `FontLoaded == 0`. Do not use
`TextBoxID`, `CurrentMenuItem`, or `MaxMenuItem` as the deciding liveness signal;
they may be retained in diagnostics.

In `skill/dialogue_recovery_real_test.go`, turn the old-man half of the probe
into permanent assertions for guarded recovery, its measured budget, deposit
tile, and no immediate retrigger. Keep its approach coordinate test-local.

### Step 3: Reuse the existing input loop

Extract `advanceUntil`'s body into one internal core accepting:

- a positive completion predicate;
- optional `stopBeforeA(*state.Mem) bool`;
- a fixed budget;
- an A-press count in its result.

Keep the existing `advanceUntil` wrapper so story/heal/gym behavior does not
drift. `dialogue_recovery.go` calls the shared core with
`state.DecodeTwoOptionMenu(mem) != nil` as `stopBeforeA`; it must not grow a
second A/StepFrame loop.

Define:

```go
type DialogueRecoveryStop uint8

const (
	DialogueRecovered DialogueRecoveryStop = iota
	DialogueChoiceRequired
	DialogueBudgetExhausted
	DialogueUnexpectedMode
)

type DialogueRecoveryResult struct {
	Stop    DialogueRecoveryStop
	Text    string
	Presses int
	Final   state.GameState
	Sprites []state.SpriteState
}

func RecoverDialogue(m *emu.Emu, budget int) DialogueRecoveryResult
```

`RecoverDialogue` is valid only after `ErrDialogueInterrupted`. It never sends
directions. Before each A press it snapshots and checks the tilemap-backed
choice decoder. Choice returns without input.

Put the exact movement/recovery invariant from Task 3 above this loop too.

### Step 4: Add deterministic boundary tests

Use a small loop seam for snapshot, tap A, and step frame. Prove:

- `WalkPath` still sends no A on dialogue interruption;
- recovery sends A only after its interruption entry point;
- the tilemap choice shape is checked before every A and receives zero input;
- stale `TextBoxID = 0x14` plus ordinary text still advances normally;
- exhaustion returns `DialogueBudgetExhausted`;
- battle/unexpected mode returns `DialogueUnexpectedMode`.

Do not add recovery knowledge to `WalkPath` or `StepOnce`.

### Step 5: Verify and commit

```bash
go test ./red/state ./skill -run 'TestDecodeTwoOptionMenu|TestDialogueRecovery|TestWalkPath' -count=1
go test ./skill -run TestViridianOldManDialogueMeasurement -count=1 -v
test ! -e skill/zz_dialogue_choice_probe_test.go
git add red/state/choice.go red/state/choice_test.go skill/dialogue_recovery.go skill/dialogue_recovery_test.go skill/dialogue_recovery_real_test.go skill/story.go skill/move.go
git commit -m "feat: stabilize interrupted dialogue safely"
```

## Task 7: Normalize raw failures with terminal overrides

**Files:**

- Create: `agent/outcome.go`
- Create: `agent/outcome_test.go`
- Modify: `agent/objective_result.go`

### Step 1: Test the three actions and default-stop policy

Keep the action mapping small:

```go
func actionFor(out Outcome) runAction {
	switch out {
	case OutcomeCompleted:
		return actionContinue
	case OutcomeBlocked:
		return actionReplan
	case OutcomeChoiceRequired:
		return actionChoice
	default:
		return actionStop
	}
}
```

Test those three exceptions directly, then one loop asserting that every
current terminal label plus an unrecognized future value uses the default stop.
The labels remain useful diagnostics; they do not need nine bespoke action
rows.

Add precedence cases:

- `ErrReplanExhausted` wins even with `ErrLegUnwalkable` in the chain;
- raw `ErrBattle`/`ErrBattleInterrupted` become ownership failure;
- `ErrMenuStuck`/`ErrCutsceneTimeout` become controller uncertainty;
- unknown errors stay terminal even when the observation looks controllable.

Run:

```bash
go test ./agent -run 'TestOutcomePolicy|TestNormalize' -count=1
```

### Step 2: Implement normalization below Run

Add:

```go
func normalizeObjectiveFailure(m *emu.Emu, partial ObjectiveResult, err error) (ObjectiveResult, error)
```

Ordering is load-bearing:

1. Preserve `Execute`'s postcondition-failed/unavailable result.
2. Check `ErrReplanExhausted` before `ErrLegUnwalkable`.
3. Map escaped battle errors to `OutcomeOwnershipFailure`.
4. Map menu/cutscene uncertainty to `OutcomeControllerUncertain`.
5. For `ErrDialogueInterrupted`, call `skill.RecoverDialogue` and translate its four stops.
6. For known gameplay blockage (`world.ErrNoPath`, `world.ErrNoRoute`, `ErrLegUnwalkable`, `*ErrBlocked`, `ErrNoDialogue`), refresh `Final` and allow `OutcomeBlocked` only when `Final.Controllable && !Final.InBattle`.
7. Everything else is `OutcomeUnknownFailure`.

Keep the original typed error chain for diagnostics. Planner summaries are short facts: objective, normalized outcome, dialogue text when available, and final named map/tile. They never prescribe the next move.

### Step 3: Verify and commit

```bash
go test ./agent -run 'TestOutcomePolicy|TestNormalize' -count=1
go test ./agent ./skill ./world -count=1
git add agent/outcome.go agent/outcome_test.go agent/objective_result.go
git commit -m "feat: normalize objective failures"
```

## Task 8: Feed outcomes to planners and preserve the stuck guard

**Files:**

- Modify: `agent/planner.go`
- Modify: `agent/planner_test.go`
- Modify: `agent/llm.go`
- Modify: `agent/llm_test.go`
- Modify: `agent/run.go`
- Modify: `agent/run_test.go`
- Create: `agent/stuck_test.go`

### Step 1: Change planner input to explicit context

```go
type PlanningContext struct {
	Observation Observation
	Recent      []ObjectiveResult
}

type Planner interface {
	Next(ctx PlanningContext, offered []Objective) (Objective, error)
}
```

`ScriptedPlanner` ignores context. LLM prompt tests assert recent result summaries and outcome codes, not only chosen names.

Migrate the current `LLMPlanner.recent []string` work to caller-supplied context. Remove planner-owned history so `Run` is the single source of truth and an exact planning context is replayable.

### Step 2: Preserve the existing no-progress guard

Add a ROM-free alternating-objective regression: objective A and objective B
alternate while every resulting progress observation is unchanged. The single
existing counter must reach `StuckAfter` because it is not keyed by objective.
Also keep the existing same-objective stuck test and prove actual progress
resets the counter.

Do not add a repeated-outcome counter. Consecutive identical result
observations become no-progress rounds after their first occurrence, so a
second counter adds no new loop class. Compute the objective + outcome + final
progress fingerprint only for the log.

Run and require failure:

```bash
go test ./agent -run 'TestScriptedPlanner|TestLLMPlanner|TestNoProgressGuard|TestAlternatingObjectivesStillStop' -count=1
```

### Step 3: Update Run

Each round:

1. Build `PlanningContext` from latest observation plus bounded recent results.
2. Offer and choose.
3. Call `Execute`.
4. On error, call `normalizeObjectiveFailure`.
5. Append every normalized `ObjectiveResult` to `Result.Outcomes`; append `Completed` only for completed outcomes.
6. Continue for `OutcomeCompleted`, re-plan for `OutcomeBlocked`, stop cleanly on `OutcomeChoiceRequired` until an explicit choice-owning objective exists, and stop with `StopError` for terminal outcomes.
7. Update the existing no-progress counter regardless of objective/outcome.
8. Stop with `StopStuck` when it reaches `Budget.StuckAfter`.
9. Keep round/frame budgets as outer bounds.

Add `StopChoiceRequired` rather than labeling a safe choice refusal a controller error. Bound history to six results and copy the slice before passing it to a planner.

Replace `Run`'s old comment with the real invariant: recoverable failures are
normalized only after the controllable-overworld gate, terminal overrides win,
and the unchanged no-progress guard bounds re-planning.

### Step 4: Improve diagnostics

Per-round logs include outcome, short summary, and the diagnostic fingerprint
of objective + outcome + resulting progress observation. Navigation failures
include named map/tile, decoded sprites, last blocker set, normalized result,
and original typed error. Only the short summary enters the model prompt.

### Step 5: Verify and commit

```bash
go test ./agent -run 'TestRun|TestScriptedPlanner|TestLLMPlanner|TestNoProgressGuard|TestAlternatingObjectivesStillStop' -count=1
go test ./agent ./skill ./world -count=1
git add agent/planner.go agent/planner_test.go agent/llm.go agent/llm_test.go agent/run.go agent/run_test.go agent/stuck_test.go
git commit -m "feat: replan from normalized outcomes"
```

## Task 9: Re-run real journeys with useful failure evidence

**Files:**

- Modify: `skill/goto_test.go`
- Modify: `skill/gym_test.go`
- Modify: `skill/dialogue_recovery_real_test.go`
- Do not stage any `skill/zz_*_test.go` scratch measurement. Repository
  convention treats those as throwaway files; their owner removes them.

### Step 1: Add milestone diagnostics, not timing assertions

Add a test helper that dumps:

```text
map name/id, player tile, controllable, in battle,
decoded sprite slots/tiles, current blocker set,
objective outcome/summary, typed error chain
```

Do not assert a particular collision. Sprite timing is nondeterministic; the milestone proves completion.

### Step 2: Repeat Route 1, the gate, and Pewter

```bash
go test ./skill -run 'TestGoToViridianPokecenter|TestTravel.*Viridian' -count=5 -v
go test ./skill -run TestViridianOldManDialogueMeasurement -count=5 -v
go test ./skill -run 'Test.*Pewter|Test.*Gym' -count=3 -v
```

Expected: Route 1 reaches Viridian, dialogue recovery returns to the measured safe tile without answering a choice, and edge-keyed routing plus live blockers complete the Pewter journey. Failures must print the diagnostic bundle; fix deterministic defects before proceeding.

### Step 3: Run final verification

```bash
go fmt ./...
go vet ./...
go test ./...
make test
git diff --check
```

If a machine lacks the ROM, skipped fixture tests are legitimate locally, but completion still requires a separately recorded `make test` run in the configured ROM environment.

### Step 4: Commit milestone tests

```bash
git add skill/goto_test.go skill/gym_test.go skill/dialogue_recovery_real_test.go
git commit -m "test: verify recovered Viridian to Pewter journey"
```

## Deferred work

Do not add a `World` cache, ROM-hash key, campaign objective, capability profile, or region graph. Do not eagerly expose `MapHeader.Signs` or `Objects` to the planner. Promote `places` to landmark records only when a concrete new objective needs an availability prerequisite, approach rule, or semantic postcondition; that future consumer is what justifies a shared `World` owner.
