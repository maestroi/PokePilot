# Stationary-object Component Re-entry Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make GoTo retain observed stationary-object component splits long enough to select a valid fresh-component border crossing.

**Architecture:** Add a Red-owned current-map topology overlay containing only positively observed `MovementStay` objects. Carry immutable overlay snapshots across legs of one GoTo call and replace a map's snapshot when it is observed again; keep moving sprites and cross-objective state out of routing.

**Tech Stack:** Go, GomeBoy save-state replay, Pokémon Red ROM adapter, `world.Graph.WithMapGrid`.

---

### Task 1: Pin observed stationary blocker selection

**Files:**
- Modify: `skill/blockers.go`
- Create: `skill/blockers_test.go`

**Step 1: Write the failing test**

Add a pure test around a helper that intersects ROM `MovementStay` home tiles
with live sprite tiles. Assert that a visible stationary trainer is included,
a moving sprite is excluded, and a hidden stationary object is excluded.

**Step 2: Run test to verify it fails**

Run: `go test ./skill -run '^TestObservedStationaryObjectBlockers$' -count=1`

Expected: FAIL because the helper does not exist.

**Step 3: Write minimal implementation**

Implement the pure intersection helper and a thin emulator-backed wrapper that
supplies `spriteBlockers(m)`.

**Step 4: Run test to verify it passes**

Run: `go test ./skill -run '^TestObservedStationaryObjectBlockers$' -count=1`

Expected: PASS.

### Task 2: Preserve map topology observations within GoTo

**Files:**
- Modify: `skill/goto.go`
- Create: `skill/goto_route13_reentry_test.go`

**Step 1: Write the failing test**

Using the private ROM, build the Route 13 grid, apply the observed stationary
trainer blocker, overlay it on the route graph, and assert the Route 14 ->
Route 13 row-8 band is a fresh component relative to `(11,4)`. Add a focused
test seam that shows a later overlay retains the Route 13 classification.

**Step 2: Run test to verify it fails**

Run: `POKEMON_RED_ROM=/home/maestro/Downloads/pokemon_red.gb go test ./skill -run '^TestRoute13StationaryTrainerMakesRow8FreshReentry$' -count=1 -v`

Expected: FAIL because GoTo's topology overlay omits the trainer or is rebuilt
from the base graph.

**Step 3: Write minimal implementation**

Before `WithMapGrid`, mark positively observed stationary object tiles
unwalkable. Keep the returned graph snapshot across GoTo iterations instead of
calling `WithMapGrid` on the immutable base each time. Reloading a map applies
a fresh replacement overlay.

**Step 4: Run focused tests**

Run: `POKEMON_RED_ROM=/home/maestro/Downloads/pokemon_red.gb go test ./skill ./world -run 'TestObservedStationaryObjectBlockers|TestRoute13StationaryTrainerMakesRow8FreshReentry|TestVisitedMapPreferenceAllowsRoute12GateComponentBridge|TestWithMapGrid' -count=1 -v`

Expected: PASS.

### Task 3: Replay the production failure and verify broad gates

**Files:**
- No committed replay state; use `/tmp/run-33v7-round3.state`.

**Step 1: Run the exact failed-state replay**

Run a temporary `skill/zz_*_test.go` that loads the captured state and calls
`TravelFlee` for Celadon City. Delete the scratch test after verification.

Expected: PASS, final map Celadon City at its declared Place, controllable and
outside battle.

**Step 2: Run focused packages**

Run: `go test ./world ./skill ./agent -count=1`

Expected: PASS.

**Step 3: Run the repository short gate**

Run: `make test-short`

Expected: navigation packages pass. If Pokewall again fails only because
`POKEPILOT_S3_ACCESS_KEY` is absent, report the already-approved baseline
exception verbatim.

**Step 4: Inspect scope**

Run: `git status --short && git diff --check && git diff --stat`

Expected: only the design/plan, focused tests, and minimal blocker/GoTo files
are changed; no ROM, `.state`, `.ram`, `.sav`, or `skill/zz_*` file is present.
