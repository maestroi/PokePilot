# Immediate Route Reversal Suppression Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Prevent `skill.GoTo` from immediately undoing a successful map transition while preserving all existing local routing and safety behavior.

**Architecture:** Track the map left by the last successful `Traverse`. Before each new map-route search, add every edge from the current map back to that previous map to the existing first-hop block set; the world graph and all permanent geometry remain unchanged.

**Tech Stack:** Go 1.26, existing `world.Graph`/`FindRouteAvoiding`, standard `testing`, real Pokémon Red ROM verification.

---

### Task 1: Pin the Route 2 reversal

**Files:**
- Create: `skill/goto_reverse_test.go`

**Step 1: Write the failing pure regression**

```go
package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestBlockImmediateReverseChoosesForestDetour(t *testing.T) {
	north := world.Edge{Kind: world.EdgeConnection, From: 0x0D, To: 0x02, Dir: 0}
	reverseLeft := world.Edge{Kind: world.EdgeWarp, From: 0x32, To: 0x0D, WarpX: 4, WarpY: 7}
	reverseRight := world.Edge{Kind: world.EdgeWarp, From: 0x32, To: 0x0D, WarpX: 5, WarpY: 7}
	forest := world.Edge{Kind: world.EdgeWarp, From: 0x32, To: 0x33, WarpX: 5, WarpY: 0}
	forestNorth := world.Edge{Kind: world.EdgeWarp, From: 0x33, To: 0x2F, WarpX: 1, WarpY: 0}
	northGate := world.Edge{Kind: world.EdgeWarp, From: 0x2F, To: 0x0D, WarpX: 5, WarpY: 0}
	g := &world.Graph{Edges: map[uint8][]world.Edge{
		0x32: {reverseLeft, reverseRight, forest},
		0x33: {forestNorth},
		0x2F: {northGate},
		0x0D: {north},
		0x02: nil,
	}}

	without, err := world.FindRouteAvoiding(g, 0x32, 0x02, nil)
	if err != nil {
		t.Fatalf("unblocked route: %v", err)
	}
	if len(without) != 2 || without[0] != reverseLeft {
		t.Fatalf("premise: unblocked route = %+v, want immediate reverse then north", without)
	}

	blocked := map[world.Edge]bool{}
	blockImmediateReverse(g, blocked, 0x32, 0x0D)
	if !blocked[reverseLeft] || !blocked[reverseRight] {
		t.Fatalf("paired reverse edges not both blocked: %+v", blocked)
	}
	if blocked[forest] {
		t.Fatalf("forward forest edge was blocked: %+v", blocked)
	}

	got, err := world.FindRouteAvoiding(g, 0x32, 0x02, blocked)
	if err != nil {
		t.Fatalf("route with immediate reverse blocked: %v", err)
	}
	want := []world.Edge{forest, forestNorth, northGate, north}
	if len(got) != len(want) {
		t.Fatalf("route = %+v, want forest detour %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("route[%d] = %+v, want %+v (route %+v)", i, got[i], want[i], got)
		}
	}
}
```

**Step 2: Verify RED**

Run:

```bash
go test ./skill -run '^TestBlockImmediateReverse' -count=1 -v
```

Expected: build failure because `blockImmediateReverse` is undefined.

### Task 2: Integrate local reversal suppression

**Files:**
- Modify: `skill/goto.go`
- Test: `skill/goto_reverse_test.go`

**Step 1: Add the helper**

```go
func blockImmediateReverse(g *world.Graph, blocked map[world.Edge]bool, current, previous uint8) {
	for _, e := range g.Edges[current] {
		if e.To == previous {
			blocked[e] = true
		}
	}
}
```

**Step 2: Track the previous map in `GoTo`**

Before the route loop:

```go
	var previousMap uint8
	havePreviousMap := false
```

After building `blockedHere` from existing per-tile failures:

```go
		if havePreviousMap {
			blockImmediateReverse(g, blockedHere, cur, previousMap)
		}
```

After a successful `Traverse`, before observing the navigation guard:

```go
		previousMap, havePreviousMap = e.From, true
```

Do not set the previous map on failed traversals. The existing error paths,
battle behavior, and local failure bans remain unchanged.

**Step 3: Verify GREEN**

Run:

```bash
go test ./skill -run '^(TestBlockImmediateReverse|TestNavigationGuard.*)$' -count=1 -v
```

Expected: all selected pure tests pass.

**Step 4: Run focused ROM-backed navigation tests**

```bash
POKEMON_RED_ROM=/home/maestro/Documents/projects/llm-gameboy/roms/pokemon_red.gb \
go test ./skill -run '^(TestBlockImmediateReverse|TestNavigationGuard.*|TestWarpTarget.*|TestTraverseGateWarp|TestTraverseWarpChain|TestTravelRecoversFromBattleOnWalkToWarp)$' -count=1 -v
```

Expected: all selected tests pass.

**Step 5: Run the complete ROM-backed suite**

```bash
POKEMON_RED_ROM=/home/maestro/Documents/projects/llm-gameboy/roms/pokemon_red.gb go test ./...
```

Expected: every package passes.

**Step 6: Run the user's workload**

Run the same configured `make run-llm` entry point with a bounded external
timeout and capture verbose output to a file. Confirm the navigation trace does
not repeat `0x32(4,7) -> 0x0d -> 0x32(4,7)` and that the run either makes real
progress beyond the south gate or stops for a different bounded gameplay
reason. Do not classify process health alone as success.

**Step 7: Commit**

```bash
git add skill/goto.go skill/goto_reverse_test.go
git commit -m "skill: prevent immediate route reversal"
```

### Task 3: Verify delivery

Run:

```bash
git branch --show-current
git status --short
git log -4 --oneline
```

Expected: `main`, clean tree, implementation commit at `HEAD`.
