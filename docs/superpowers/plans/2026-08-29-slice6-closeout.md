# Slice 6 Close-Out Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the four debts slice 6 left behind: the Caterpie→Butterfree acceptance test that never ran, a battle prompt answered by reflex, a false fact committed to RUNNOTES, and a fixture cache that is rebuilt from scratch on every worker run.

**Architecture:** Four independent tasks against the existing `skill` package. No new subsystems. Task 1 is infrastructure and makes every later task minutes faster, so it goes first. Task 2 fixes a `Battle` state-machine gap that Task 3's test would otherwise trip over. Task 3 is the slice-6 acceptance test that S6-4 declared impossible on a false premise. Task 4 is documentation repair.

**Tech Stack:** Go, GomeBoy headless emulator (`emu.Emu`), Pokemon Red RAM decoding (`red/state`), pokered decompilation at `~/.cache/pokered` as the source of ROM truth.

**Spec:** No separate spec doc. This plan is self-contained: every fact it rests on was measured or read from the decomp on 2026-08-29 and is quoted inline with its source. The measurements behind it are in the S6-3 / S6-4 RUNNOTES sections, recoverable at `f5300305:RUNNOTES.md` and `8a64e468:RUNNOTES.md`.

## Global Constraints

Copied verbatim from the slice 6 plan description; every task's requirements include these.

- A predicate asserts something POSITIVE about the state you want, never merely the absence of what you do not want.
- Menus are step-and-verify: press, assert `wCurrentMenuItem` reached the wanted index, then A. Never a press count, never a frame count.
- Coordinates come from `skill.Place`, never literals.
- Never commit a ROM or any `.gb`/`.sav`/`.state` file.
- A stated fact that turns out to be false must be REPORTED, not worked around. That is always allowed and is never a reason to spend the whole budget.
- Under agent-runner: **DO NOT COMMIT.** Leave edits uncommitted; a clean tree trips the no-changes gate. (The `git commit` steps below apply only when a human runs this plan directly.)
- **RUNNOTES.md: APPEND your section. Do not replace the file.** Five slice-6 tasks in a row wiped their predecessors' measurements off the tip.
- The ROM is at `/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb`, passed as `POKEMON_RED_ROM`.

### Worker-safety rules (this plan is executed unattended by a 27B model)

Three slice-6 runs died on these, so they are constraints, not advice:

- **Do NOT open `pokered/` unless a task explicitly tells you to, and then only the exact file named.** S6-4 attempt 1 and S6-8 attempt 1 each burned their entire runtime reading the decomp. Every ROM fact you need is inlined below.
- **Do NOT run `git log` or read collision grids.**
- **If a command returns the same output twice in a row, STOP repeating it and move to the next step.** S6-3 attempt 1 issued the same `sed` 205 times and produced nothing.
- Write code before running long tests. A test run that rebuilds a fixture takes minutes; that is expected, not a hang.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `skill/fixture/fixture.go` | `Dir` becomes an env-overridable var so the fixture cache can live outside the worktree | 1 |
| `skill/fixture/fixture_test.go` | Test for the override and its default | 1 |
| `skill/battle.go` | New `case` for the "make room for a new move?" prompt | 2 |
| `skill/battle_test.go` | Unit test for the new predicate | 2 |
| `skill/catch_test.go` | The real Caterpie→Butterfree acceptance test | 3 |
| `RUNNOTES.md` | Corrections + this cycle's measurements | 4 |
| `docs/AGENT.md` | The Pokecenter PC fact, so it is found next time | 4 |

---

### Task 1: Fixture cache survives a fresh worktree

**Why first:** every agent-runner worktree is a clean checkout, and `*.state` plus `testdata/fixtures/` are both gitignored, so `post_pokeballs` is rebuilt from `post_starter` on every single run — starter, parcel, Route 22 training, then the balls. Measured cost: `skill/fixture` 46s and `skill` 111s cold, versus 1.7s and 59s once cached. Tasks 2 and 3 both load fixtures, so this pays for itself immediately.

**Files:**
- Modify: `skill/fixture/fixture.go` (the `const Dir` declaration, currently `const Dir = "testdata/fixtures"`)
- Test: `skill/fixture/fixture_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `fixture.DefaultDir` (untyped string const, `"testdata/fixtures"`) and `fixture.ResolveDir() string`. The old `fixture.Dir` const is REMOVED, so any code referencing `fixture.Dir` must change to `fixture.ResolveDir()` — Step 3 names the one call site and gives you the grep to find any others. `fixture.Load(t *testing.T, name string) *emu.Emu` keeps its exact signature.

- [ ] **Step 1: Write the failing test**

Add to `skill/fixture/fixture_test.go`:

```go
func TestDirHonoursEnvOverride(t *testing.T) {
	// The agent-runner gives every run a clean worktree, and the fixture
	// cache is gitignored, so without an override every run rebuilds
	// post_pokeballs from scratch. POKEPILOT_FIXTURE_DIR lets the cache
	// live outside the worktree and be shared across runs.
	t.Setenv("POKEPILOT_FIXTURE_DIR", "/tmp/pokepilot-fixtures-test")
	if got := fixture.ResolveDir(); got != "/tmp/pokepilot-fixtures-test" {
		t.Fatalf("ResolveDir() = %q, want the env override", got)
	}
}

func TestDirDefaultsToRepoPath(t *testing.T) {
	t.Setenv("POKEPILOT_FIXTURE_DIR", "")
	if got := fixture.ResolveDir(); got != "testdata/fixtures" {
		t.Fatalf("ResolveDir() = %q, want the in-repo default", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```
cd /home/maestro/Documents/projects/PokePilot && go test ./skill/fixture -run TestDir -count=1
```
Expected: FAIL to build, `undefined: fixture.ResolveDir`.

- [ ] **Step 3: Write the minimal implementation**

In `skill/fixture/fixture.go`, replace the line `const Dir = "testdata/fixtures"` with:

```go
// DefaultDir is the in-repo fixture cache, used when nothing overrides it.
// It is gitignored (.gitignore: *.state and testdata/fixtures/).
const DefaultDir = "testdata/fixtures"

// ResolveDir is where generated fixtures are cached.
//
// POKEPILOT_FIXTURE_DIR overrides it. This exists because agent-runner gives
// every run a CLEAN worktree and the cache is gitignored, so without an
// override each run rebuilds post_pokeballs from post_starter — starter,
// parcel, Route 22 training, balls — before any new code runs. MEASURED
// 2026-08-29: skill/fixture 46s cold vs 1.7s warm, skill 111s cold vs 59s
// warm. Point the variable at a directory OUTSIDE the worktree to share the
// cache between runs.
func ResolveDir() string {
	if d := os.Getenv("POKEPILOT_FIXTURE_DIR"); d != "" {
		return d
	}
	return DefaultDir
}
```

Add `"os"` to the import block if it is not already there.

Then replace every use of the old `Dir` identifier with a `ResolveDir()` call. There is exactly one at the time of writing, in `fixturePath`:

```go
func fixturePath(name string) string {
	return filepath.Join(ResolveDir(), fmt.Sprintf("%s.v%d.state", name, fixtureVersion))
}
```

Find any others with:
```
cd /home/maestro/Documents/projects/PokePilot && grep -rn "fixture\.Dir\|[^a-zA-Z]Dir\b" skill/ --include="*.go"
```
Leave `FailureDir` alone — it is a different constant and is not part of this task.

- [ ] **Step 4: Run the tests to verify they pass**

Run:
```
cd /home/maestro/Documents/projects/PokePilot && go build ./... && go vet ./... && go test ./skill/fixture -run TestDir -count=1 -v
```
Expected: both tests PASS.

- [ ] **Step 5: Prove the cache is actually reused**

Run twice, pointing at a scratch directory, and compare the times:

```
mkdir -p /tmp/pokepilot-fixtures
cd /home/maestro/Documents/projects/PokePilot
POKEPILOT_FIXTURE_DIR=/tmp/pokepilot-fixtures POKEMON_RED_ROM=/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb \
  go test ./skill/fixture -count=1 2>&1 | tail -2
POKEPILOT_FIXTURE_DIR=/tmp/pokepilot-fixtures POKEMON_RED_ROM=/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb \
  go test ./skill/fixture -count=1 2>&1 | tail -2
```
Expected: the second run is dramatically faster than the first, and `/tmp/pokepilot-fixtures` now holds `.state` files. Record both numbers — they go in RUNNOTES in Task 4.

- [ ] **Step 6: Commit** (human runs only; under agent-runner, skip)

```bash
git add skill/fixture/fixture.go skill/fixture/fixture_test.go
git commit -m "fixture: POKEPILOT_FIXTURE_DIR so the cache survives a clean worktree"
```

---

### Task 2: Battle stops answering the "make room for a new move?" prompt by reflex

**The bug, MEASURED by S6-4 on 2026-08-29:** a level-22 SQUIRTLE with four moves was offered BITE. `Train` did not hang and kept grinding, but BITE was NOT learned — the moveset stayed `[33 39 145 55]`. No mon with a full moveset can currently learn anything, and the answer is incidental rather than chosen.

**The ROM truth, read from `~/.cache/pokered/engine/pokemon/learn_move.asm`, `TryingToLearn`. Do not go re-read it; this is the whole of what matters:**

```
TryingToLearn:
	ld hl, TryingToLearnText
	call PrintText
	hlcoord 14, 7
	ld a, TWO_OPTION_MENU
	ld [wTextBoxID], a
	call DisplayTextBoxID ; yes/no menu
	ld a, [wCurrentMenuItem]
	rra
	ret c                  ; wCurrentMenuItem == 1 (NO) -> carry -> AbandonLearning
	...                    ; wCurrentMenuItem == 0 (YES) -> "Which move to forget?" list
```

So: index 0 = YES (opens a four-item move list), index 1 = NO (abandons). The cursor defaults to YES, because `DisplayTwoOptionMenu` only starts on the second option when `BIT_SECOND_MENU_OPTION_DEFAULT` is set and `TryingToLearn` does not set it.

`skill/battle.go`'s state machine has cases for `useNextMonUp` and `partyMenuUp`; anything else falls to `default:`, which does `m.Tap(emu.A, 3, 7)`. This prompt has no case, so it is answered incidentally. **This is the same failure family as the S6-3 nickname prompt** — a yes/no answered by reflex.

**The decision, and why:** decline deliberately. Choosing YES means picking a move to delete, and this line's whole point is CONFUSION on a Butterfree — deleting the wrong slot silently ruins a run, which is exactly what the S6-4 task text warned about. Declining is safe, matches the currently observed behaviour, and becomes a *chosen, reported* outcome instead of an accident. Replacing a specific slot is a later task with its own test; do not build it here (YAGNI).

**Files:**
- Modify: `skill/battle.go` (the `switch` in the battle loop, and a new predicate near `useNextMonUp`)
- Test: `skill/battle_test.go`

**Interfaces:**
- Consumes: `state.DecodeTwoOptionMenu(*state.Mem) *state.TwoOptionMenu` and `SelectMenuItem(m *emu.Emu, index int) error`, both already used by `battle.go`.
- Produces: `func learnMovePromptUp(m *emu.Emu) bool` — unexported predicate, true when the "make room" yes/no is on screen. Nothing else changes.

**Do NOT add a counter for this.** `Battle` returns `(state.BattleResult, error)`, and `state.BattleResult` is a **uint8 enum** (`ResultWon`/`ResultLost`/`ResultDraw`), not a struct — there is nowhere to thread a count without changing a signature that `travel.go`, `train.go`, `gym.go` and `catch.go` all call. Declining is silent. The observable postcondition is the moveset itself, which Task 3 asserts directly. If a count is ever genuinely needed, that is its own task with its own reason.

- [ ] **Step 1: Write the failing test**

Add to `skill/battle_test.go`. This is a screen-shape test against `wTileMap`, matching how the other battle predicates are tested — copy the surrounding file's helper style rather than inventing one:

```go
func TestLearnMovePromptUpDetectsTheMakeRoomBox(t *testing.T) {
	// TryingToLearn (learn_move.asm) prints "…wants to learn…" then opens a
	// TWO_OPTION_MENU at hlcoord 14,7. Index 0 is YES (pick a move to
	// delete), index 1 is NO. The prompt must be recognised so Battle can
	// DECLINE it deliberately instead of answering it with a reflex A.
	m := battleFixtureWithText(t, "wants to learn")
	if !learnMovePromptUp(m) {
		t.Fatal("learnMovePromptUp = false, want true when the make-room prompt is up")
	}
}

func TestLearnMovePromptUpIgnoresOrdinaryBattleText(t *testing.T) {
	m := battleFixtureWithText(t, "used TACKLE")
	if learnMovePromptUp(m) {
		t.Fatal("learnMovePromptUp = true on ordinary battle text, want false")
	}
}
```

If `battleFixtureWithText` does not exist, read the existing tests in `skill/battle_test.go` and reuse whatever helper they already use to stage `wTileMap`. Do not write an emulator-driving test here — this predicate is pure RAM shape.

- [ ] **Step 2: Run the test to verify it fails**

Run:
```
cd /home/maestro/Documents/projects/PokePilot && go test ./skill -run TestLearnMovePromptUp -count=1
```
Expected: FAIL to build, `undefined: learnMovePromptUp`.

- [ ] **Step 3: Write the minimal implementation**

In `skill/battle.go`, next to the other predicates:

```go
// learnMovePromptUp reports whether the "make room for a new move?" yes/no is
// on screen. TryingToLearn (pokered/engine/pokemon/learn_move.asm) prints its
// text and opens a TWO_OPTION_MENU at hlcoord 14,7 whose index 0 is YES —
// which opens a "which move to forget?" list — and index 1 is NO.
//
// It must be its own case. Left to the default branch, a reflex A press
// answers a question about DELETING a move, and on this project's line that
// move can be CONFUSION. MEASURED 2026-08-29: a level-22 SQUIRTLE was offered
// BITE and its moveset came back unchanged, so the answer was already being
// made by accident rather than chosen.
func learnMovePromptUp(m *emu.Emu) bool {
	var s state.Mem
	state.Snapshot(m, &s)
	if state.DecodeTwoOptionMenu(&s) == nil {
		return false
	}
	return battleTextContains(m, "wants to learn")
}
```

Use whatever the file's existing screen-text helper is called; find it with:
```
cd /home/maestro/Documents/projects/PokePilot && grep -n "func battleText\|ScreenText\|wTileMap" skill/battle.go | head
```
If no text helper exists, use the same `wTileMap` decode the neighbouring predicates use.

Then add the case to the battle `switch`, **before** `default:`:

```go
case learnMovePromptUp(m):
	// Decline: index 1 is NO. Saying YES opens a move list, and picking a
	// slot blind can delete CONFUSION, which is the move this project's
	// Butterfree line exists to get. Declining is a CHOSEN outcome, not an
	// accident; replacing a specific slot deliberately is a separate task
	// with its own test.
	if err := SelectMenuItem(m, 1); err != nil {
		return menuError(m, "decline the make-room prompt", err)
	}
```

That is the whole change. No counter, no signature change, no edit to `train.go`.

- [ ] **Step 4: Run the tests to verify they pass**

Run:
```
cd /home/maestro/Documents/projects/PokePilot && go build ./... && go vet ./... && go test ./skill -run TestLearnMovePromptUp -count=1 -v
```
Expected: both PASS.

- [ ] **Step 5: Verify nothing else regressed**

Run:
```
cd /home/maestro/Documents/projects/PokePilot && POKEMON_RED_ROM=/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb go test ./... -count=1 -short -timeout 10m
```
Expected: all packages `ok`.

- [ ] **Step 6: Commit** (human runs only)

```bash
git add skill/battle.go skill/battle_test.go
git commit -m "battle: decline the make-room-for-a-move prompt deliberately, not by reflex"
```

---

### Task 3: The Caterpie→Butterfree acceptance test S6-4 never ran

**What S6-4 was asked for and did not deliver:** *"train a caught Caterpie to 12 and assert the party member's species is BUTTERFREE and its moves include CONFUSION. Species alone is not enough; the move is the point."* It substituted a SQUIRTLE test and passed, because the gate only greps `-run TestTrain`. It reported the substitution honestly.

**Why it said the real test was impossible, and why that reason was wrong.** S6-4's RUNNOTES states there is no way to make the Caterpie the sole party member because *"this decompilation's pokecenters have no PC machine sprite."* **That is false.** Every Pokemon Center has a working PC; it is not a sprite, which is why a sprite search missed it. From `~/.cache/pokered/data/events/hidden_events.asm`:

```
	hidden_events_for VIRIDIAN_POKECENTER
	hidden_event  0,  4, PrintBenchGuyText,   SPRITE_FACING_LEFT
	hidden_event 13,  3, OpenPokemonCenterPC, SPRITE_FACING_UP
```
The same two entries appear for `PEWTER_POKECENTER` and `CERULEAN_POKECENTER`.

**But you do not need the PC.** The real blocker S6-4 measured correctly was that a weak Caterpie faints while a healthy partner is still alive, the ROM asks "Who will fight?", and `battle.go` could not answer. **S6-5b closed exactly that gap** — `skill/party.go` now has `SelectPartySlot`, and `battle.go` has a `partyMenuUp` case that sends out `firstLivePartySlot`. So the two-mon party is fine now: let the Caterpie faint, let the partner finish the fight.

**The remaining problem is EXP, and this is the part to think about.** In Gen 1 a fainted mon earns no experience, and `PromoteToLead` only reorders the party. A level-3 Caterpie put in front of Route 1 wilds faints fast. So the grind must keep the Caterpie alive, not just present. Use `skill.Train` with the Caterpie as lead and a **small** `maxBattles` per session, healing between sessions — the same bounded-session shape `TestGymBoulderBadge` uses. If the Caterpie cannot survive to 12 this way, that is a REAL FINDING: report it with numbers in RUNNOTES and stop. Do not invent a new mechanism, and do not lower the target from 12.

**Facts, all settled — do NOT go verify them:**
- `CATERPIE` = `0x7B`, `METAPOD` = `0x7C`, `BUTTERFREE` = `0x7D`, `CONFUSION` = move id `0x5D` (93).
- Evolutions are at level **7** (Metapod) and **10** (Butterfree). CONFUSION is learned at **12**.
- At level 12 the mon holds exactly three moves — TACKLE, STRING_SHOT, CONFUSION — with a free fourth slot, so the "make room" prompt from Task 2 **never fires on this line**. Verified from the decomp: evolution calls `LearnMoveFromLevelUp`, which matches on EXACT level (`cp b ; is the move learnt at the mon's current level?`), Metapod's and Butterfree's learnsets are empty below 12, and Metapod's HARDEN is a level-1 *base* move granted by `WriteMonMoves` on generation, never on evolution.
- `Train` already survives the evolution cutscene unchanged: it runs while `wIsInBattle` is still set, so `Battle`'s loop advances it. S6-4 proved this with a SQUIRTLE to level 22.

**Files:**
- Modify: `skill/catch_test.go` (add one test; leave `TestCatchCaterpie` alone)
- Test: same file

**Interfaces:**
- Consumes: `skill.Catch(m, romData, want []uint8, policy, maxBalls) (CatchResult, error)`; `skill.Train(m, romData, targetLevel int, policy, maxBattles int) (TrainResult, error)`; `skill.PromoteToLead(m *emu.Emu, index int) error`; `skill.Heal(m *emu.Emu) error`; `skill.Travel(m, romData, dest, policy, maxBattles) (…, error)`; `skill.Place(name string) (Destination, bool)`; `state.DecodeParty(*state.Mem)`; `fixture.Load(t, "post_pokeballs")`.
- Produces: `TestCatchButterfreeLine`, the slice-6 acceptance test. Nothing consumes it.

- [ ] **Step 1: Write the failing test**

Add to `skill/catch_test.go`:

```go
// speciesButterfree and moveConfusion are the postcondition of the whole
// slice: a Charmander run beats Brock with a Butterfree's CONFUSION, because
// Rock resists Fire and Onix's Special is its weak stat.
const (
	speciesButterfree uint8 = 0x7D
	moveConfusion     uint8 = 0x5D
)

// TestCatchButterfreeLine is the acceptance test S6-4 was asked for and did
// not run: catch a Caterpie, train it to 12, and assert it is a BUTTERFREE
// that KNOWS CONFUSION. Species alone is not enough; the move is the point.
//
// The Caterpie is trained as party LEAD with a healthy partner behind it, so
// a faint is survivable: S6-5b taught Battle to answer the ROM's "Who will
// fight?" by sending out the first live slot. Sessions are short and the
// party is healed between them, the same bounded shape TestGymBoulderBadge
// uses, because a level-3 Caterpie dies quickly in Route 1 grass.
func TestCatchButterfreeLine(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey: catch in Viridian Forest, then grind to level 12")
	}

	m := fixture.Load(t, "post_pokeballs")
	romData := m.ROM()
	policy := skill.StatAwareMove(romData)

	forest, ok := skill.Place("viridian forest")
	if !ok {
		t.Fatal(`Place "viridian forest" not found`)
	}
	if _, err := skill.Travel(m, romData, forest, policy, 20); err != nil {
		t.Fatalf("travel to Viridian Forest: %v", err)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	partyBefore := int(state.DecodeParty(&mem).Count)

	res, err := skill.Catch(m, romData, []uint8{speciesCaterpie}, policy, 5)
	if err != nil {
		t.Fatalf("Catch: %v (result %+v)", err, res)
	}
	if res.Outcome != skill.OutcomeCaught {
		t.Skipf("no Caterpie caught this run (outcome=%d balls=%d encounters=%d); "+
			"the hunt misses ~19%% of the time by design, this is not a failure of the line",
			res.Outcome, res.BallsThrown, res.Encounters)
	}

	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	if int(party.Count) != partyBefore+1 {
		t.Fatalf("party is %d after a reported catch, want %d", party.Count, partyBefore+1)
	}
	caught := int(party.Count) - 1

	// Put the Caterpie in front so it earns the experience: a fainted or
	// benched mon earns none in Gen 1.
	if err := skill.PromoteToLead(m, caught); err != nil {
		t.Fatalf("PromoteToLead(%d): %v", caught, err)
	}

	center, ok := skill.Place("viridian pokemon center")
	if !ok {
		t.Fatal(`Place "viridian pokemon center" not found`)
	}

	const (
		targetLevel      = 12
		sessionBattles   = 3
		maxGrindSessions = 20
	)
	totalBattles := 0
	for session := 0; session < maxGrindSessions; session++ {
		state.Snapshot(m, &mem)
		lead := state.DecodeParty(&mem).Mons[0]
		if int(lead.Level) >= targetLevel {
			break
		}
		tr, err := skill.Train(m, romData, targetLevel, policy, sessionBattles)
		if err != nil {
			t.Fatalf("Train session %d: %v (result %+v)", session+1, err, tr)
		}
		totalBattles += tr.Battles

		state.Snapshot(m, &mem)
		lead = state.DecodeParty(&mem).Mons[0]
		if int(lead.Level) < targetLevel && (lead.HP*3 < lead.MaxHP || lead.Status != 0) {
			if _, err := skill.Travel(m, romData, center, policy, 10); err != nil {
				t.Fatalf("travel to heal after session %d: %v", session+1, err)
			}
			if err := skill.Heal(m); err != nil {
				t.Fatalf("Heal after session %d: %v", session+1, err)
			}
			if _, err := skill.Travel(m, romData, forest, policy, 20); err != nil {
				t.Fatalf("travel back to the forest after session %d: %v", session+1, err)
			}
		}
	}

	state.Snapshot(m, &mem)
	lead := state.DecodeParty(&mem).Mons[0]
	t.Logf("after %d battles: species=%#02x level=%d moves=%v",
		totalBattles, lead.Species, lead.Level, lead.Moves)

	if int(lead.Level) < targetLevel {
		t.Fatalf("the caught mon reached only level %d in %d battles, want %d — "+
			"REPORT this with the numbers, do not lower the target",
			lead.Level, totalBattles, targetLevel)
	}
	if lead.Species != speciesButterfree {
		t.Fatalf("species is %#02x at level %d, want BUTTERFREE (%#02x)",
			lead.Species, lead.Level, speciesButterfree)
	}
	var knowsConfusion bool
	for _, mv := range lead.Moves {
		if mv == moveConfusion {
			knowsConfusion = true
		}
	}
	if !knowsConfusion {
		t.Fatalf("BUTTERFREE at level %d does not know CONFUSION (%#02x); moves=%v — "+
			"the move is the point of this line, not the species",
			lead.Level, moveConfusion, lead.Moves)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails for the right reason**

Run:
```
cd /home/maestro/Documents/projects/PokePilot && go vet ./skill
```
Expected: PASS. This test does not depend on Task 2 compiling — it asserts the moveset directly. Run Task 2 first anyway: on this line the make-room prompt never fires (three moves, free slot), but Task 2 is what stops a reflex A press from deleting a move if that assumption is ever wrong.

- [ ] **Step 3: Run the acceptance test**

Run (expect 10-25 minutes; the fixture may rebuild if Task 1's cache is cold):
```
cd /home/maestro/Documents/projects/PokePilot && \
POKEMON_RED_ROM=/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb \
POKEPILOT_FIXTURE_DIR=/tmp/pokepilot-fixtures \
  go test ./skill -run TestCatchButterfreeLine -count=1 -v -timeout 45m > /tmp/butterfree.log 2>&1; echo "exit=$?"
grep -nE "^--- |^ok |^FAIL|species=|after [0-9]+ battles" /tmp/butterfree.log
```

- [ ] **Step 4: Read the result honestly**

Three outcomes, three different responses:
- **PASS** — the line is proven. Record the numbers from the log.
- **SKIP** ("no Caterpie caught this run") — the hunt missed. Re-run once. If it skips twice, record that and say the acceptance test is hunt-limited, not line-limited.
- **FAIL** — record the exact failure line. If it is the level check, report the level reached and the battle count and STOP. Do not lower `targetLevel`, do not raise `maxGrindSessions` past 20, and do not add a mechanism. A Caterpie that cannot survive to 12 is a finding this slice needs, not a budget to widen.

- [ ] **Step 5: Confirm the short suite is untouched**

Run:
```
cd /home/maestro/Documents/projects/PokePilot && POKEMON_RED_ROM=/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb go test ./... -count=1 -short -timeout 10m
```
Expected: all `ok`, and `TestCatchButterfreeLine` does not run (it is `-short`-skipped).

- [ ] **Step 6: Commit** (human runs only)

```bash
git add skill/catch_test.go
git commit -m "skill: the Caterpie->Butterfree acceptance test S6-4 did not run"
```

---

### Task 4: Correct the record

Three things slice 6 committed are wrong or lost. This task fixes the documentation only — no behaviour changes.

**Files:**
- Modify: `RUNNOTES.md` (APPEND a section; do not rewrite the file)
- Modify: `docs/AGENT.md` (add the PC fact where a future task will find it)

**Interfaces:** none.

- [ ] **Step 1: Append the corrections to RUNNOTES.md**

Append — do not replace — this section:

```markdown
## Slice 6 close-out — corrections to the record

### FALSE FACT, committed in S6-4's RUNNOTES: "pokecenters have no PC"
S6-4 concluded its acceptance test was impossible because "this decompilation's
pokecenters have **no PC machine sprite**". That is wrong. Every Pokemon Center
has a working PC. It is not a sprite — which is why a sprite search missed it —
it is a tile-activated hidden event:

    ~/.cache/pokered/data/events/hidden_events.asm
        hidden_events_for VIRIDIAN_POKECENTER
        hidden_event  0, 4, PrintBenchGuyText,   SPRITE_FACING_LEFT
        hidden_event 13, 3, OpenPokemonCenterPC, SPRITE_FACING_UP
    (same two entries for PEWTER_POKECENTER and CERULEAN_POKECENTER)
    data/text_predef_pointers.asm:37  add_tx_pre PokemonCenterPCText ; 1F

The PC is at tile (13,3) of every Center, activated by standing on it and
FACING UP. Depositing a mon IS possible in-rom. Nothing in this project needs
it yet — the actual blocker S6-4 hit was the "Who will fight?" gap, which
S6-5b closed — but no future task should design around "there is no PC".

### The "make room for a new move?" prompt was answered by accident
MEASURED by S6-4: a level-22 SQUIRTLE was offered BITE and its moveset came
back unchanged, so no mon with four moves could learn anything. Fixed in this
close-out: Battle now has an explicit case that DECLINES the prompt via
SelectMenuItem(m, 1). Index 0 is YES (opens a "which move to forget?" list)
and index 1 is NO —
pokered/engine/pokemon/learn_move.asm, TryingToLearn. This is the same failure
family as the S6-3 nickname prompt: a yes/no answered by reflex.

### RUNNOTES was being wiped every task
Five slice-6 tasks in a row REPLACED this file instead of appending, so each
task's measurements vanished from the tip. S6-6 nearly shipped a guess because
its instructions said "read S6-3's RUNNOTES numbers" and those numbers were no
longer there. Earlier sections are recoverable in git:
    S6-0f  82880c7:RUNNOTES.md
    S6-3   f5300305:RUNNOTES.md
    S6-4   8a64e468:RUNNOTES.md
APPEND to this file. Do not replace it.

### Fixture cache
POKEPILOT_FIXTURE_DIR now overrides the gitignored in-repo cache, so an
agent-runner worktree can share fixtures instead of rebuilding post_pokeballs
from post_starter every run. Fill in the two measured timings from Task 1
Step 5 here: cold ___s, warm ___s.
```

Replace the two blanks with the real numbers from Task 1 Step 5. Do not invent them.

- [ ] **Step 2: Add the PC fact to docs/AGENT.md**

Append to whatever "ROM facts" or equivalent section `docs/AGENT.md` already has (read the file first and match its heading style):

```markdown
- **Pokemon Center PC:** tile (13,3) of every Center, faced UP. It is a
  `hidden_event` (`OpenPokemonCenterPC`), NOT an NPC sprite, so searching the
  map object files for a PC sprite finds nothing and is misleading.
```

- [ ] **Step 3: Verify nothing broke**

Run:
```
cd /home/maestro/Documents/projects/PokePilot && go build ./... && go vet ./... && POKEMON_RED_ROM=/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb go test ./... -count=1 -short -timeout 10m
```
Expected: all `ok`. This task changes no Go code, so a failure here means something from an earlier task is broken.

- [ ] **Step 4: Commit** (human runs only)

```bash
git add RUNNOTES.md docs/AGENT.md
git commit -m "docs: correct the Pokecenter PC claim and record the slice 6 close-out"
```

---

## Task ordering

1 → 2 → 3 → 4. Tasks 1, 2 and 3 are independently compilable — no task depends on another's symbols — but run them in order: Task 1 makes 2 and 3 measurably faster, Task 2 removes a reflex-A hazard before Task 3 grinds through 20 sessions, and Task 4 records numbers produced by Tasks 1 and 3.

## What is deliberately NOT in this plan

- **Replacing a specific move slot.** Task 2 declines the prompt; choosing which move to delete needs a policy and its own test, and nothing needs it yet.
- **A PC deposit skill.** The fact is corrected so nobody designs around a false constraint, but no current task needs to deposit a mon.
- **Widening `catchHuntCap` / `catchGrassLegs`.** The hunt misses ~19% of the time because Caterpie is 5.1% of Viridian Forest encounters. That is measured and reported; widening a budget to make a stochastic search pass is what the slice 6 plan bans.
- **Anything S6-12 turns up.** Its scoreboard should be read before the next slice is designed, and its findings merged with this close-out.
