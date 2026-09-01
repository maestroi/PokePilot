# Navigation Stall Guard Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Bound every `skill.GoTo` call so repeated route states or excessive successful map transitions return a typed error instead of running forever.

**Architecture:** Add a small pure `navigationGuard` in `skill/goto.go`. `GoTo` records its initial `(map, x, y)` and observes the settled state after each successful `Traverse`; the guard rejects an exact repeat and the 65th successful transition. Existing route search, fresh sprite blockers, battle handling, and per-tile failed-leg bans remain unchanged.

**Tech Stack:** Go 1.26, standard `errors`/`fmt`/`strings`, existing `testing`, existing ROM-backed skill suite.

---

### Task 1: Specify the pure navigation guard

**Files:**
- Create: `skill/goto_guard_test.go`

**Step 1: Write the failing tests**

Create package-local tests covering exact repetition, legitimate same-map re-entry at a different tile, and the hard transition ceiling:

```go
package skill

import (
	"errors"
	"strings"
	"testing"
)

func TestNavigationGuardRejectsRepeatedState(t *testing.T) {
	dest := Destination{Map: 0x02, X: 14, Y: 8}
	start := navigationState{Map: 0x0D, X: 3, Y: 43}
	g := newNavigationGuard(dest, start)

	gate := navigationState{Map: 0x32, X: 4, Y: 7}
	if err := g.observe(gate); err != nil {
		t.Fatalf("first transition: %v", err)
	}
	err := g.observe(start)
	if !errors.Is(err, ErrNavigationStalled) {
		t.Fatalf("repeat error = %v, want ErrNavigationStalled", err)
	}
	for _, want := range []string{"map 0d at (3,43)", "map 02 at (14,8)", "2 transition", "0d(3,43) -> 32(4,7) -> 0d(3,43)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestNavigationGuardAllowsSameMapAtDifferentTile(t *testing.T) {
	g := newNavigationGuard(Destination{Map: 0x02}, navigationState{Map: 0x0D, X: 3, Y: 43})
	for _, s := range []navigationState{
		{Map: 0x32, X: 4, Y: 7},
		{Map: 0x33, X: 17, Y: 47},
		{Map: 0x2F, X: 4, Y: 7},
		{Map: 0x0D, X: 3, Y: 11},
	} {
		if err := g.observe(s); err != nil {
			t.Fatalf("observe %+v: %v", s, err)
		}
	}
}

func TestNavigationGuardBoundsSuccessfulTransitions(t *testing.T) {
	g := newNavigationGuard(Destination{Map: 0xF0}, navigationState{})
	for i := 1; i <= maxNavigationTransitions; i++ {
		if err := g.observe(navigationState{Map: uint8(i)}); err != nil {
			t.Fatalf("transition %d: %v", i, err)
		}
	}
	err := g.observe(navigationState{Map: maxNavigationTransitions + 1})
	if !errors.Is(err, ErrNavigationStalled) {
		t.Fatalf("transition ceiling error = %v, want ErrNavigationStalled", err)
	}
	if !strings.Contains(err.Error(), "exceeded 64 successful map transitions") {
		t.Fatalf("ceiling error = %q", err)
	}
}
```

**Step 2: Run the tests to verify RED**

Run:

```bash
go test ./skill -run '^TestNavigationGuard' -count=1 -v
```

Expected: build failure because `navigationState`, `newNavigationGuard`, `ErrNavigationStalled`, and `maxNavigationTransitions` do not exist.

**Step 3: Commit the test specification only after the RED result is recorded**

Do not commit yet; proceed directly to Task 2 so `main` is not left with an intentionally uncompilable test.

### Task 2: Implement and integrate the guard

**Files:**
- Modify: `skill/goto.go`
- Test: `skill/goto_guard_test.go`

**Step 1: Add the minimal guard implementation**

Add near the existing navigation errors:

```go
var ErrNavigationStalled = errors.New("skill: navigation made no progress")

const maxNavigationTransitions = 64

type navigationState struct {
	Map  uint8
	X, Y uint8
}

type navigationGuard struct {
	dest        Destination
	seen        map[navigationState]bool
	trace       []navigationState
	transitions int
}

func newNavigationGuard(dest Destination, start navigationState) *navigationGuard {
	return &navigationGuard{
		dest:  dest,
		seen:  map[navigationState]bool{start: true},
		trace: []navigationState{start},
	}
}

func (g *navigationGuard) observe(now navigationState) error {
	g.transitions++
	g.trace = append(g.trace, now)
	if g.seen[now] {
		return fmt.Errorf("%w: repeated map %02x at (%d,%d) after %d transitions toward map %02x at (%d,%d); trace: %s",
			ErrNavigationStalled, now.Map, now.X, now.Y, g.transitions,
			g.dest.Map, g.dest.X, g.dest.Y, formatNavigationTrace(g.trace))
	}
	if g.transitions > maxNavigationTransitions {
		return fmt.Errorf("%w: exceeded %d successful map transitions at map %02x (%d,%d) toward map %02x at (%d,%d); trace: %s",
			ErrNavigationStalled, maxNavigationTransitions, now.Map, now.X, now.Y,
			g.dest.Map, g.dest.X, g.dest.Y, formatNavigationTrace(g.trace))
	}
	g.seen[now] = true
	return nil
}

func formatNavigationTrace(trace []navigationState) string {
	parts := make([]string, len(trace))
	for i, s := range trace {
		parts[i] = fmt.Sprintf("%02x(%d,%d)", s.Map, s.X, s.Y)
	}
	return strings.Join(parts, " -> ")
}
```

Add `strings` to the imports.

**Step 2: Integrate it into `GoTo`**

After `BuildGraph` succeeds, read the initial state and construct the guard:

```go
	startX, startY := playerXY(m)
	guard := newNavigationGuard(dest, navigationState{
		Map: m.Peek8(sym.CurMap), X: startX, Y: startY,
	})
```

After each successful `Traverse`, observe the newly settled state:

```go
		nowX, nowY := playerXY(m)
		if err := guard.observe(navigationState{
			Map: m.Peek8(sym.CurMap), X: nowX, Y: nowY,
		}); err != nil {
			return fmt.Errorf("skill: GoTo: %w", err)
		}
```

This block belongs after the existing `if err := Traverse(...); err != nil`
branch, so failed traversals and battles do not count as successful map
transitions.

**Step 3: Run the pure guard tests to verify GREEN**

Run:

```bash
go test ./skill -run '^TestNavigationGuard' -count=1 -v
```

Expected: all three tests pass.

**Step 4: Run focused navigation regressions**

Run:

```bash
POKEMON_RED_ROM=/home/maestro/Documents/projects/llm-gameboy/roms/pokemon_red.gb \
go test ./skill -run '^(TestNavigationGuard|TestWarpTarget|TestTraverseGateWarp|TestTraverseWarpChain|TestTravelRecoversFromBattleOnWalkToWarp)$' -count=1 -v
```

Expected: all selected tests pass. Route 2 re-entry at a different tile must
not produce `ErrNavigationStalled`.

**Step 5: Run the complete ROM-backed suite**

Run:

```bash
POKEMON_RED_ROM=/home/maestro/Documents/projects/llm-gameboy/roms/pokemon_red.gb go test ./...
```

Expected: every package passes; intentionally skipped journey milestones remain skipped.

**Step 6: Commit**

```bash
git add skill/goto.go skill/goto_guard_test.go
git commit -m "skill: bound stalled navigation"
```

### Task 3: Verify delivery on main

**Files:**
- Verify only

**Step 1: Check commit and tree state**

Run:

```bash
git branch --show-current
git status --short
git log -2 --oneline
```

Expected: branch is `main`, working tree is clean, and the design plus implementation commits are at `HEAD`.
