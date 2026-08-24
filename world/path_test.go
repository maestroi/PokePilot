package world

import (
	"errors"
	"testing"
)

// testGrid builds a fully walkable w x h grid with no ROM.
func testGrid(w, h int) *Grid {
	g := &Grid{Width: w, Height: h, walkable: make([]bool, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g.Set(x, y, true)
		}
	}
	return g
}

// replayPath applies each step from (sx,sy) in order, failing the test if
// any position leaves the grid, lands on a solid tile, or lands on a tile
// in blocked. It returns the final position.
func replayPath(t *testing.T, g *Grid, sx, sy int, blocked map[[2]int]bool, steps []Step) (int, int) {
	t.Helper()
	x, y := sx, sy
	for i, s := range steps {
		x += s.DX
		y += s.DY
		if !g.InBounds(x, y) {
			t.Fatalf("step %d (%s) leaves the grid at (%d,%d)", i, s, x, y)
		}
		if !g.Walkable(x, y) {
			t.Fatalf("step %d (%s) lands on solid tile (%d,%d)", i, s, x, y)
		}
		if blocked[[2]int{x, y}] {
			t.Fatalf("step %d lands on blocked tile (%d,%d)", i, x, y)
		}
	}
	return x, y
}

func TestFindPathStraightLine(t *testing.T) {
	g := testGrid(5, 5)

	steps, err := FindPath(g, 0, 0, 0, 3, nil)
	if err != nil {
		t.Fatalf("FindPath: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("len(steps) = %d, want 3: %v", len(steps), steps)
	}
	for i, s := range steps {
		if s != StepDown {
			t.Errorf("steps[%d] = %s, want down", i, s)
		}
	}
	if x, y := replayPath(t, g, 0, 0, nil, steps); x != 0 || y != 3 {
		t.Fatalf("replay ended at (%d,%d), want (0,3)", x, y)
	}
}

func TestFindPathAroundWall(t *testing.T) {
	g := testGrid(5, 5)
	for _, y := range []int{0, 1, 2} {
		g.Set(1, y, false)
	}

	steps, err := FindPath(g, 0, 0, 2, 0, nil)
	if err != nil {
		t.Fatalf("FindPath: %v", err)
	}
	if len(steps) <= 2 {
		t.Fatalf("path length %d is not longer than the 2-step direct route: %v", len(steps), steps)
	}
	// replayPath fails the test if any step lands on a non-walkable tile.
	if x, y := replayPath(t, g, 0, 0, nil, steps); x != 2 || y != 0 {
		t.Fatalf("replay ended at (%d,%d), want (2,0)", x, y)
	}
}

func TestFindPathNoRoute(t *testing.T) {
	g := testGrid(5, 5)
	for _, p := range [][2]int{{1, 2}, {3, 2}, {2, 1}, {2, 3}} {
		g.Set(p[0], p[1], false)
	}

	_, err := FindPath(g, 0, 0, 2, 2, nil)
	if !errors.Is(err, ErrNoPath) {
		t.Fatalf("err = %v, want ErrNoPath", err)
	}
}

func TestFindPathSameTile(t *testing.T) {
	g := testGrid(5, 5)

	steps, err := FindPath(g, 2, 2, 2, 2, nil)
	if err != nil {
		t.Fatalf("FindPath: %v", err)
	}
	if steps == nil {
		t.Fatal("steps = nil, want non-nil empty slice")
	}
	if len(steps) != 0 {
		t.Fatalf("len(steps) = %d, want 0", len(steps))
	}
}

func TestFindPathRespectsBlocked(t *testing.T) {
	g := testGrid(5, 5)
	blocked := map[[2]int]bool{{1, 0}: true}

	steps, err := FindPath(g, 0, 0, 2, 0, blocked)
	if err != nil {
		t.Fatalf("FindPath: %v", err)
	}
	if len(steps) <= 2 {
		t.Fatalf("path length %d does not detour around the blocked tile: %v", len(steps), steps)
	}
	// replayPath fails the test if any step lands on the blocked tile.
	if x, y := replayPath(t, g, 0, 0, blocked, steps); x != 2 || y != 0 {
		t.Fatalf("replay ended at (%d,%d), want (2,0)", x, y)
	}
}

func TestPathIsWalkable(t *testing.T) {
	g := testGrid(5, 5)
	for _, y := range []int{0, 1, 2} {
		g.Set(1, y, false)
	}

	steps, err := FindPath(g, 0, 0, 2, 0, nil)
	if err != nil {
		t.Fatalf("FindPath: %v", err)
	}
	// Replaying the returned steps in order must land exactly on the
	// destination, through walkable tiles only.
	if x, y := replayPath(t, g, 0, 0, nil, steps); x != 2 || y != 0 {
		t.Fatalf("replay ended at (%d,%d), want (2,0)", x, y)
	}
}

func TestFindPathOutOfBounds(t *testing.T) {
	g := testGrid(5, 5)

	_, err := FindPath(g, 0, 0, 7, 7, nil)
	if !errors.Is(err, ErrNoPath) {
		t.Fatalf("err = %v, want ErrNoPath", err)
	}
}
