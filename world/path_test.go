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

func TestFindPathFromSolidStart(t *testing.T) {
	tests := []struct {
		name      string
		solid     [][2]int
		sx, sy    int
		dx, dy    int
		wantFirst Step // zero value means any step that leaves the start
	}{
		{
			name:  "solid corner start",
			solid: [][2]int{{0, 0}},
			sx:    0,
			sy:    0,
			dx:    4,
			dy:    4,
		},
		{
			name:      "solid start with one open side",
			solid:     [][2]int{{2, 2}, {2, 1}, {1, 2}, {3, 2}},
			sx:        2,
			sy:        2,
			dx:        4,
			dy:        4,
			wantFirst: StepDown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := testGrid(5, 5)
			for _, p := range tt.solid {
				g.Set(p[0], p[1], false)
			}

			steps, err := FindPath(g, tt.sx, tt.sy, tt.dx, tt.dy, nil)
			if err != nil {
				t.Fatalf("FindPath from solid start: %v", err)
			}
			if len(steps) == 0 {
				t.Fatal("no steps returned from a solid start")
			}
			if tt.wantFirst != (Step{}) && steps[0] != tt.wantFirst {
				t.Errorf("first step = %s, want %s", steps[0], tt.wantFirst)
			}
			// replayPath fails if any landing tile is solid, so the path
			// must leave the solid start and never return to it.
			if x, y := replayPath(t, g, tt.sx, tt.sy, nil, steps); x != tt.dx || y != tt.dy {
				t.Fatalf("replay ended at (%d,%d), want (%d,%d)", x, y, tt.dx, tt.dy)
			}
			for _, p := range tt.solid {
				if g.Walkable(p[0], p[1]) {
					t.Errorf("tile (%d,%d) became walkable after FindPath", p[0], p[1])
				}
			}
		})
	}
}

func TestFindPathAdjacent(t *testing.T) {
	tests := []struct {
		name     string
		solid    [][2]int
		sx, sy   int
		tx, ty   int
		wantErr  bool
		wantEnd  [2]int // final path position: the neighbour of the target
		wantPush Step
		wantLen  int
	}{
		{
			name:     "push into solid warp",
			solid:    [][2]int{{2, 2}},
			sx:       0,
			sy:       0,
			tx:       2,
			ty:       2,
			wantEnd:  [2]int{2, 1},
			wantPush: StepDown,
			wantLen:  3,
		},
		{
			name:     "walkable target still works",
			sx:       0,
			sy:       0,
			tx:       2,
			ty:       2,
			wantEnd:  [2]int{2, 1},
			wantPush: StepDown,
			wantLen:  3,
		},
		{
			name:     "start already next to the warp",
			solid:    [][2]int{{2, 2}},
			sx:       1,
			sy:       2,
			tx:       2,
			ty:       2,
			wantEnd:  [2]int{1, 2},
			wantPush: StepRight,
			wantLen:  0,
		},
		{
			name:     "solid start next to a solid warp",
			solid:    [][2]int{{1, 1}, {3, 3}},
			sx:       1,
			sy:       1,
			tx:       3,
			ty:       3,
			wantEnd:  [2]int{3, 2},
			wantPush: StepDown,
			wantLen:  3,
		},
		{
			name:    "all neighbours solid",
			solid:   [][2]int{{2, 2}, {2, 1}, {1, 2}, {3, 2}, {2, 3}},
			sx:      0,
			sy:      0,
			tx:      2,
			ty:      2,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := testGrid(5, 5)
			for _, p := range tt.solid {
				g.Set(p[0], p[1], false)
			}

			steps, push, err := FindPathAdjacent(g, tt.sx, tt.sy, tt.tx, tt.ty, nil)
			if tt.wantErr {
				if !errors.Is(err, ErrNoPath) {
					t.Fatalf("err = %v, want ErrNoPath", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindPathAdjacent: %v", err)
			}
			if len(steps) != tt.wantLen {
				t.Fatalf("len(steps) = %d, want %d: %v", len(steps), tt.wantLen, steps)
			}
			if push != tt.wantPush {
				t.Errorf("push = %s, want %s", push, tt.wantPush)
			}
			// The path must end exactly on the chosen neighbour, through
			// walkable tiles only.
			if x, y := replayPath(t, g, tt.sx, tt.sy, nil, steps); x != tt.wantEnd[0] || y != tt.wantEnd[1] {
				t.Fatalf("replay ended at (%d,%d), want neighbour (%d,%d)", x, y, tt.wantEnd[0], tt.wantEnd[1])
			}
			// The push step must move from the neighbour into the target.
			if nx, ny := tt.wantEnd[0]+push.DX, tt.wantEnd[1]+push.DY; nx != tt.tx || ny != tt.ty {
				t.Fatalf("push from (%d,%d) lands at (%d,%d), want target (%d,%d)", tt.wantEnd[0], tt.wantEnd[1], nx, ny, tt.tx, tt.ty)
			}
			for _, p := range tt.solid {
				if g.Walkable(p[0], p[1]) {
					t.Errorf("tile (%d,%d) became walkable after FindPathAdjacent", p[0], p[1])
				}
			}
		})
	}
}

func TestPathCallsDoNotMutateGrid(t *testing.T) {
	g := testGrid(5, 5)
	g.Set(0, 0, false) // solid start tile
	g.Set(2, 2, false) // solid warp tile

	if _, err := FindPath(g, 0, 0, 4, 4, nil); err != nil {
		t.Fatalf("FindPath: %v", err)
	}
	if _, _, err := FindPathAdjacent(g, 0, 0, 2, 2, nil); err != nil {
		t.Fatalf("FindPathAdjacent: %v", err)
	}

	if g.Walkable(0, 0) {
		t.Error("solid start tile (0,0) became walkable")
	}
	if g.Walkable(2, 2) {
		t.Error("solid warp tile (2,2) became walkable")
	}
	if !g.Walkable(4, 4) {
		t.Error("walkable tile (4,4) became solid")
	}
}
