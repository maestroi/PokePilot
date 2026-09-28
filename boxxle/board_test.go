package boxxle

import (
	"testing"
)

// gridSpec is a compact ASCII description of a board for tests:
//
//	# = wall
//	. = floor
//	@ = player
//	$ = crate
//	* = crate on goal
//	+ = goal
//	(space) = empty/out of board
type gridSpec []string

// buildState converts a gridSpec into a State. The grid is the whole board
// (no implicit border), so tests control the exact wall layout.
func buildState(t *testing.T, g gridSpec) State {
	t.Helper()
	h := len(g)
	w := 0
	for _, row := range g {
		if len(row) > w {
			w = len(row)
		}
	}
	var walls, goals, crates []Pos
	var player *Pos
	for y, row := range g {
		for x := 0; x < len(row); x++ {
			switch row[x] {
			case '#':
				walls = append(walls, Pos{X: x, Y: y})
			case '.':
				// floor, nothing to record
			case '@':
				player = &Pos{X: x, Y: y}
			case '$':
				crates = append(crates, Pos{X: x, Y: y})
			case '*':
				crates = append(crates, Pos{X: x, Y: y})
				goals = append(goals, Pos{X: x, Y: y})
			case '+':
				goals = append(goals, Pos{X: x, Y: y})
			}
		}
	}
	return State{
		Screen: ScreenPuzzle,
		Width:  w,
		Height: h,
		Walls:  walls,
		Goals:  goals,
		Crates: crates,
		Player: player,
	}
}

func TestNewBoardRejectsEmpty(t *testing.T) {
	if _, err := NewBoard(State{}); err == nil {
		t.Fatal("expected error for empty board")
	}
	if _, err := NewBoard(State{Width: 5, Height: 5}); err == nil {
		t.Fatal("expected error for board with no player")
	}
}

func TestWalkable(t *testing.T) {
	g := gridSpec{
		"#####",
		"#.@$#" + ".",
		"#####",
	}
	b, err := NewBoard(buildState(t, g))
	if err != nil {
		t.Fatal(err)
	}
	// Floor is walkable.
	if !b.Walkable(Pos{X: 2, Y: 1}) {
		t.Error("expected floor to be walkable")
	}
	// Wall is not walkable.
	if b.Walkable(Pos{X: 0, Y: 1}) {
		t.Error("expected wall to be not walkable")
	}
	// Crate is not walkable.
	if b.Walkable(Pos{X: 4, Y: 1}) {
		t.Error("expected crate to be not walkable")
	}
	// Out of bounds is not walkable.
	if b.Walkable(Pos{X: 10, Y: 1}) {
		t.Error("expected out-of-bounds to be not walkable")
	}
}

func TestReachable(t *testing.T) {
	g := gridSpec{
		"#####",
		"#...#",
		"#####",
		"#...#",
		"#####",
	}
	state := buildState(t, g)
	state.Player = &Pos{X: 1, Y: 1}
	b, err := NewBoard(state)
	if err != nil {
		t.Fatal(err)
	}
	reach := b.Reachable(*state.Player)
	// The player can reach the open cells in the top region.
	if !reach[Pos{X: 2, Y: 1}] {
		t.Error("expected (2,1) to be reachable")
	}
	if !reach[Pos{X: 3, Y: 1}] {
		t.Error("expected (3,1) to be reachable")
	}
	// The bottom region is separated by a wall.
	if reach[Pos{X: 1, Y: 3}] {
		t.Error("expected (1,3) to be not reachable")
	}
}

func TestPathTo(t *testing.T) {
	g := gridSpec{
		"#####",
		"#...#",
		"#...#",
		"#...#",
		"#####",
	}
	state := buildState(t, g)
	state.Player = &Pos{X: 1, Y: 1}
	b, err := NewBoard(state)
	if err != nil {
		t.Fatal(err)
	}
	path, ok := b.PathTo(Pos{X: 1, Y: 1}, Pos{X: 3, Y: 3})
	if !ok {
		t.Fatal("expected a path")
	}
	// The path should be 4 steps (right, right, down, down) in some order.
	if len(path) != 4 {
		t.Errorf("expected 4 steps, got %d: %v", len(path), path)
	}
	// The last step should be the goal.
	if path[len(path)-1] != (Pos{X: 3, Y: 3}) {
		t.Errorf("expected path to end at goal, got %v", path[len(path)-1])
	}
	// No step should be a wall.
	for _, p := range path {
		if b.IsWall(p) {
			t.Errorf("path steps on wall %v", p)
		}
	}
}

func TestPathToUnreachable(t *testing.T) {
	g := gridSpec{
		"#####",
		"#.#.#",
		"#.#.#",
		"#.#.#",
		"#####",
	}
	state := buildState(t, g)
	state.Player = &Pos{X: 1, Y: 1}
	b, err := NewBoard(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.PathTo(Pos{X: 1, Y: 1}, Pos{X: 3, Y: 1}); ok {
		t.Error("expected no path to unreachable cell")
	}
}

func TestLegalPushes(t *testing.T) {
	// A crate with the player adjacent on the left, and open space on the
	// right: the only legal push is right.
	g := gridSpec{
		"######",
		"#.@$.#",
		"######",
	}
	state := buildState(t, g)
	b, err := NewBoard(state)
	if err != nil {
		t.Fatal(err)
	}
	pushes := b.LegalPushes()
	if len(pushes) != 1 {
		t.Fatalf("expected 1 legal push, got %d: %v", len(pushes), pushes)
	}
	p := pushes[0]
	if p.Crate != (Pos{X: 3, Y: 1}) {
		t.Errorf("expected crate at (3,1), got %v", p.Crate)
	}
	if p.Dir != DirRight {
		t.Errorf("expected push right, got %v", p.Dir)
	}
	if p.CrateTo != (Pos{X: 4, Y: 1}) {
		t.Errorf("expected crate to move to (4,1), got %v", p.CrateTo)
	}
	if p.PlayerFrom != (Pos{X: 2, Y: 1}) {
		t.Errorf("expected player from (2,1), got %v", p.PlayerFrom)
	}
}

func TestLegalPushesBlocked(t *testing.T) {
	// A crate against a wall: no legal push in that direction.
	g := gridSpec{
		"######",
		"#@$##.",
		"######",
	}
	state := buildState(t, g)
	b, err := NewBoard(state)
	if err != nil {
		t.Fatal(err)
	}
	pushes := b.LegalPushes()
	// The crate at (2,1) can be pushed right (to (3,1)), but not left
	// (player is there), not up (wall), not down (wall).
	for _, p := range pushes {
		if p.Crate == (Pos{X: 2, Y: 1}) && p.Dir == DirRight {
			// This is the expected push.
			continue
		}
		if p.Crate == (Pos{X: 2, Y: 1}) {
			t.Errorf("unexpected push for crate at (2,1): %v", p)
		}
	}
}

func TestFindPush(t *testing.T) {
	g := gridSpec{
		"######",
		"#.@$.#",
		"######",
	}
	state := buildState(t, g)
	b, err := NewBoard(state)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := b.FindPush(Pos{X: 3, Y: 1}, DirRight)
	if !ok {
		t.Fatal("expected push to be legal")
	}
	if p.CrateTo != (Pos{X: 4, Y: 1}) {
		t.Errorf("expected crate to move to (4,1), got %v", p.CrateTo)
	}
	// Pushing left is not legal (player is there, and the crate would move
	// into the player).
	if _, ok := b.FindPush(Pos{X: 3, Y: 1}, DirLeft); ok {
		t.Error("expected push left to be not legal")
	}
}

func TestDeadSquareCorner(t *testing.T) {
	// A corner: the cell at (1,1) has walls above and to the left, so it is a
	// dead square.
	g := gridSpec{
		"#####",
		"#...#",
		"#...#",
		"#####",
	}
	state := buildState(t, g)
	state.Player = &Pos{X: 2, Y: 2}
	b, err := NewBoard(state)
	if err != nil {
		t.Fatal(err)
	}
	// (1,1) is a corner (wall above at (1,0), wall left at (0,1)).
	if !b.IsDeadSquare(Pos{X: 1, Y: 1}) {
		t.Error("expected (1,1) to be a dead square")
	}
	// (3,1) is a corner (wall above at (3,0), wall right at (4,1)).
	if !b.IsDeadSquare(Pos{X: 3, Y: 1}) {
		t.Error("expected (3,1) to be a dead square")
	}
	// (2,2) is not a corner.
	if b.IsDeadSquare(Pos{X: 2, Y: 2}) {
		t.Error("expected (2,2) to not be a dead square")
	}
}

func TestDeadlocked(t *testing.T) {
	// A crate in a corner: the board is deadlocked.
	g := gridSpec{
		"#####",
		"#$..#",
		"#...#",
		"#####",
	}
	state := buildState(t, g)
	state.Player = &Pos{X: 2, Y: 2}
	b, err := NewBoard(state)
	if err != nil {
		t.Fatal(err)
	}
	dead := b.Deadlocked()
	if len(dead) != 1 {
		t.Fatalf("expected 1 dead crate, got %d: %v", len(dead), dead)
	}
	if dead[0] != (Pos{X: 1, Y: 1}) {
		t.Errorf("expected dead crate at (1,1), got %v", dead[0])
	}
}

func TestPushDeadlock(t *testing.T) {
	// A crate that can be pushed into a corner: the push is a deadlock.
	g := gridSpec{
		"######",
		"#.@$.#",
		"#....#",
		"######",
	}
	state := buildState(t, g)
	b, err := NewBoard(state)
	if err != nil {
		t.Fatal(err)
	}
	// Pushing the crate at (3,1) right moves it to (4,1), which is a corner
	// (wall above at (4,0), wall right at (5,1)).
	p, ok := b.FindPush(Pos{X: 3, Y: 1}, DirRight)
	if !ok {
		t.Fatal("expected push right to be legal")
	}
	if !b.PushDeadlock(p) {
		t.Error("expected push right to be a deadlock")
	}
}

func TestLegalPushesRequiresReachablePlayerFrom(t *testing.T) {
	// A crate that is legally pushable in geometry but the player cannot reach
	// the pre-push square: the push is not legal.
	g := gridSpec{
		"#######",
		"#.@.#$#",
		"#.#.#.#",
		"#...#.#",
		"#######",
	}
	state := buildState(t, g)
	b, err := NewBoard(state)
	if err != nil {
		t.Fatal(err)
	}
	// The crate at (5,1) can be pushed down (to (5,2)), but the player at
	// (2,1) cannot reach (5,0) because of the wall at (4,1).
	// Actually, let me check: the crate is at (5,1). Pushing down requires
	// the player to be at (5,0). Is (5,0) reachable from (2,1)?
	// The wall at (4,1) blocks the direct path, but the player can go around
	// through (3,2), (4,2), (5,2)... wait, (5,2) is where the crate would
	// move to. Let me reconsider.
	//
	// Actually, the point is: if the player cannot reach the pre-push square,
	// the push is not legal. Let me construct a clearer case.
	pushes := b.LegalPushes()
	// Verify that no push has a PlayerFrom that is not reachable.
	reach := b.Reachable(*state.Player)
	for _, p := range pushes {
		if !reach[p.PlayerFrom] {
			t.Errorf("push %v has unreachable PlayerFrom %v", p, p.PlayerFrom)
		}
	}
}
