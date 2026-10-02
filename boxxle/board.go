// Package boxxle board geometry: pure, ROM-free reasoning over a decoded
// Boxxle (Sokoban) board.
//
// This layer owns the deterministic facts a push executor needs: which cells
// the player can walk, which crate pushes are legal, how to route the player
// to a pre-push square, and which squares are obvious dead squares. It works
// entirely on the decoded State (Walls/Goals/Crates/Player) and never touches
// RAM or emulator input, so it is testable against synthetic board fixtures.
package boxxle

// Direction is one of the four board-adjacent movement directions.
type Direction int

// The four cardinal directions. The zero value is DirUp.
const (
	DirUp Direction = iota
	DirDown
	DirLeft
	DirRight
)

// dirDeltas maps each direction to its board-cell offset.
var dirDeltas = [4]Pos{
	DirUp:    {X: 0, Y: -1},
	DirDown:  {X: 0, Y: 1},
	DirLeft:  {X: -1, Y: 0},
	DirRight: {X: 1, Y: 0},
}

// Offset returns the board-cell delta for a direction.
func (d Direction) Offset() Pos { return dirDeltas[d] }

// Valid reports whether d is one of the four cardinal directions.
func (d Direction) Valid() bool { return d >= DirUp && d <= DirRight }

// String returns a short human name for the direction.
func (d Direction) String() string {
	switch d {
	case DirUp:
		return "up"
	case DirDown:
		return "down"
	case DirLeft:
		return "left"
	case DirRight:
		return "right"
	default:
		return "invalid"
	}
}

// Push is a semantic crate push: the crate at Crate moved one cell in Dir,
// with the player stepping from PlayerFrom into the crate's old cell.
type Push struct {
	Crate      Pos
	Dir        Direction
	PlayerFrom Pos
	CrateTo    Pos
}

// Board is the immutable geometric view of a decoded State that the geometry
// functions operate on. It precomputes the wall, crate, and goal sets so that
// repeated queries are cheap.
type Board struct {
	state  State
	walls  map[Pos]bool
	crates map[Pos]bool
	goals  map[Pos]bool
}

// NewBoard builds a geometry view over a decoded State. It validates that the
// board is a visible puzzle (non-zero dimensions and a player) and returns a
// typed error otherwise, so callers never reason over a zero board.
func NewBoard(state State) (Board, error) {
	if state.Width <= 0 || state.Height <= 0 {
		return Board{}, &BoardError{Detail: "board has no dimensions"}
	}
	if state.Player == nil {
		return Board{}, &BoardError{Detail: "board has no player"}
	}
	b := Board{
		state:  state,
		walls:  make(map[Pos]bool, len(state.Walls)),
		crates: make(map[Pos]bool, len(state.Crates)),
		goals:  make(map[Pos]bool, len(state.Goals)),
	}
	for _, p := range state.Walls {
		b.walls[p] = true
	}
	for _, p := range state.Crates {
		b.crates[p] = true
	}
	for _, p := range state.Goals {
		b.goals[p] = true
	}
	return b, nil
}

// BoardError reports that a board is not usable for geometry.
type BoardError struct{ Detail string }

func (e *BoardError) Error() string { return "boxxle board: " + e.Detail }

// State returns the decoded State the board was built from.
func (b Board) State() State { return b.state }

// inBounds reports whether p is inside the board.
func (b Board) inBounds(p Pos) bool {
	return p.X >= 0 && p.X < b.state.Width && p.Y >= 0 && p.Y < b.state.Height
}

// IsWall reports whether p is a wall cell.
func (b Board) IsWall(p Pos) bool { return b.walls[p] }

// IsCrate reports whether p is a crate cell.
func (b Board) IsCrate(p Pos) bool { return b.crates[p] }

// IsGoal reports whether p is a goal cell.
func (b Board) IsGoal(p Pos) bool { return b.goals[p] }

// Walkable reports whether the player may stand on p: in bounds, not a wall,
// and not a crate. This is the positive predicate the executor uses to decide
// whether a step is possible.
func (b Board) Walkable(p Pos) bool {
	return b.inBounds(p) && !b.walls[p] && !b.crates[p]
}

// Reachable returns the set of cells the player can walk to from start,
// treating crates and walls as solid. start itself is included when it is
// walkable. The region is the positive fact the executor uses to confirm a
// pre-push square is reachable before sending any input.
func (b Board) Reachable(start Pos) map[Pos]bool {
	reach := make(map[Pos]bool)
	if !b.Walkable(start) {
		return reach
	}
	reach[start] = true
	queue := []Pos{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for d := Direction(0); d < Direction(len(dirDeltas)); d++ {
			nxt := Pos{X: cur.X + dirDeltas[d].X, Y: cur.Y + dirDeltas[d].Y}
			if reach[nxt] || !b.Walkable(nxt) {
				continue
			}
			reach[nxt] = true
			queue = append(queue, nxt)
		}
	}
	return reach
}

// PathTo returns a shortest walk from start to goal as the sequence of cells
// the player visits after start (so the first element is the first step). It
// reports false when goal is not reachable. The path never steps on a wall or
// a crate, which is the postcondition the executor relies on.
func (b Board) PathTo(start, goal Pos) ([]Pos, bool) {
	if !b.Walkable(goal) {
		return nil, false
	}
	if start == goal {
		return []Pos{}, true
	}
	parent := map[Pos]Pos{}
	visited := map[Pos]bool{start: true}
	queue := []Pos{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for d := Direction(0); d < Direction(len(dirDeltas)); d++ {
			nxt := Pos{X: cur.X + dirDeltas[d].X, Y: cur.Y + dirDeltas[d].Y}
			if visited[nxt] || !b.Walkable(nxt) {
				continue
			}
			visited[nxt] = true
			parent[nxt] = cur
			if nxt == goal {
				// Reconstruct the path from goal back to start.
				path := []Pos{}
				for step := goal; step != start; {
					path = append([]Pos{step}, path...)
					step = parent[step]
				}
				return path, true
			}
			queue = append(queue, nxt)
		}
	}
	return nil, false
}

// LegalPush describes one crate push the player can perform: stand at
// PlayerFrom, walk into the crate at Crate, and the crate moves to CrateTo.
type LegalPush struct {
	Crate      Pos
	Dir        Direction
	PlayerFrom Pos
	CrateTo    Pos
}

// LegalPushes enumerates every crate push the player can currently perform.
// A push is legal when:
//   - the crate exists at Crate,
//   - the cell behind the crate (CrateTo = Crate + Dir) is in bounds, not a
//     wall, and not another crate,
//   - the cell in front of the crate (PlayerFrom = Crate - Dir) is walkable,
//     and
//   - the player can reach PlayerFrom from the current player position.
//
// The result is deterministic: crates are iterated in State order and
// directions in a fixed order, so the same board always yields the same list.
func (b Board) LegalPushes() []LegalPush {
	var out []LegalPush
	reach := b.Reachable(*b.state.Player)
	for _, crate := range b.state.Crates {
		for d := Direction(0); d < Direction(len(dirDeltas)); d++ {
			delta := dirDeltas[d]
			crateTo := Pos{X: crate.X + delta.X, Y: crate.Y + delta.Y}
			playerFrom := Pos{X: crate.X - delta.X, Y: crate.Y - delta.Y}
			if !b.inBounds(crateTo) {
				continue
			}
			if b.walls[crateTo] || b.crates[crateTo] {
				continue
			}
			if !b.Walkable(playerFrom) {
				continue
			}
			if !reach[playerFrom] {
				continue
			}
			out = append(out, LegalPush{
				Crate:      crate,
				Dir:        d,
				PlayerFrom: playerFrom,
				CrateTo:    crateTo,
			})
		}
	}
	return out
}

// FindPush locates the specific push of the crate at Crate in Dir, if it is
// legal. It reports false when the push is not currently possible, which is
// the typed rejection the executor uses before sending any emulator input.
func (b Board) FindPush(crate Pos, dir Direction) (LegalPush, bool) {
	if !dir.Valid() {
		return LegalPush{}, false
	}
	if !b.crates[crate] {
		return LegalPush{}, false
	}
	delta := dirDeltas[dir]
	crateTo := Pos{X: crate.X + delta.X, Y: crate.Y + delta.Y}
	playerFrom := Pos{X: crate.X - delta.X, Y: crate.Y - delta.Y}
	if !b.inBounds(crateTo) || b.walls[crateTo] || b.crates[crateTo] {
		return LegalPush{}, false
	}
	if !b.Walkable(playerFrom) {
		return LegalPush{}, false
	}
	reach := b.Reachable(*b.state.Player)
	if !reach[playerFrom] {
		return LegalPush{}, false
	}
	return LegalPush{Crate: crate, Dir: dir, PlayerFrom: playerFrom, CrateTo: crateTo}, true
}

// deadSquares returns the set of cells that are obvious static dead squares:
// a cell where a crate can never reach any goal because it is pinned in a
// corner (two adjacent orthogonal walls). This is the cheap, deterministic
// deadlock guard: a crate pushed onto a dead square loses the puzzle.
//
// A corner dead square is one where the two walls meeting at that cell block
// both axes of movement. We detect the four corner patterns:
//
//	# .      . #      # #
//	# #   and   #   and   # .
//	. #      # .      . #
//
// where # is a wall (or the board edge) and . is the candidate cell.
func (b Board) deadSquares() map[Pos]bool {
	dead := make(map[Pos]bool)
	solid := func(p Pos) bool {
		if !b.inBounds(p) {
			return true // out of bounds acts as a wall
		}
		return b.walls[p]
	}
	for y := 0; y < b.state.Height; y++ {
		for x := 0; x < b.state.Width; x++ {
			p := Pos{X: x, Y: y}
			if b.walls[p] {
				continue
			}
			// A goal is never a dead square: a crate on a goal has already
			// reached its objective, even if the goal sits in a corner.
			if b.goals[p] {
				continue
			}
			// Corner: two adjacent orthogonal walls (or edges).
			// Top-left corner: wall above and wall to the left.
			if solid(Pos{X: x, Y: y - 1}) && solid(Pos{X: x - 1, Y: y}) {
				dead[p] = true
				continue
			}
			// Top-right corner: wall above and wall to the right.
			if solid(Pos{X: x, Y: y - 1}) && solid(Pos{X: x + 1, Y: y}) {
				dead[p] = true
				continue
			}
			// Bottom-left corner: wall below and wall to the left.
			if solid(Pos{X: x, Y: y + 1}) && solid(Pos{X: x - 1, Y: y}) {
				dead[p] = true
				continue
			}
			// Bottom-right corner: wall below and wall to the right.
			if solid(Pos{X: x, Y: y + 1}) && solid(Pos{X: x + 1, Y: y}) {
				dead[p] = true
			}
		}
	}
	return dead
}

// DeadSquares returns the full static dead-square set for the board, so a
// caller that checks many cells (a solver, a batch deadlock audit) computes
// the corners once instead of once per cell.
func (b Board) DeadSquares() map[Pos]bool {
	return b.deadSquares()
}

// IsDeadSquare reports whether p is an obvious static dead square (a corner
// where a crate can never reach a goal). The executor uses this to reject a
// push that would move a crate onto a dead square.
func (b Board) IsDeadSquare(p Pos) bool {
	return b.deadSquares()[p]
}

// DeadCrate reports whether the crate at p is stuck on a dead square, which
// means the puzzle is lost (that crate can never reach a goal). This is the
// positive deadlock postcondition: a board with a dead crate is deadlocked.
func (b Board) DeadCrate(p Pos) bool {
	return b.crates[p] && b.deadSquares()[p]
}

// Deadlocked reports whether the board is in an obvious deadlock: any crate
// sits on a dead square. It returns the dead crate positions so a caller can
// report which crate lost the puzzle.
func (b Board) Deadlocked() []Pos {
	var dead []Pos
	for _, c := range b.state.Crates {
		if b.deadSquares()[c] {
			dead = append(dead, c)
		}
	}
	return dead
}

// PushDeadlock reports whether performing the push would move the crate onto
// a dead square. The policy uses this to reject a push that creates an
// obvious deadlock, keeping the deterministic guard in front of the executor.
func (b Board) PushDeadlock(push LegalPush) bool {
	return b.deadSquares()[push.CrateTo]
}

// RenderBoard draws the compact ASCII board used by planners, heartbeats and
// the operator/spectator UI:
//
//	# wall, . floor, @ player, $ crate, * crate on goal, + goal.
//
// A player standing on a goal is drawn as @ so the live position is never
// hidden behind the goal mark.
func RenderBoard(state State) []string {
	if state.Width <= 0 || state.Height <= 0 {
		return nil
	}
	walls := make(map[Pos]bool, len(state.Walls))
	for _, w := range state.Walls {
		walls[w] = true
	}
	goals := make(map[Pos]bool, len(state.Goals))
	for _, g := range state.Goals {
		goals[g] = true
	}
	crates := make(map[Pos]bool, len(state.Crates))
	for _, c := range state.Crates {
		crates[c] = true
	}
	rows := make([]string, state.Height)
	for y := 0; y < state.Height; y++ {
		row := make([]byte, state.Width)
		for x := 0; x < state.Width; x++ {
			p := Pos{X: x, Y: y}
			switch {
			case walls[p]:
				row[x] = '#'
			case crates[p] && goals[p]:
				row[x] = '*'
			case crates[p]:
				row[x] = '$'
			case state.Player != nil && *state.Player == p:
				row[x] = '@'
			case goals[p]:
				row[x] = '+'
			default:
				row[x] = '.'
			}
		}
		rows[y] = string(row)
	}
	return rows
}

// CratesOnGoal counts crates that currently sit on a goal cell.
func CratesOnGoal(state State) int {
	if len(state.Crates) == 0 || len(state.Goals) == 0 {
		return 0
	}
	goals := make(map[Pos]bool, len(state.Goals))
	for _, g := range state.Goals {
		goals[g] = true
	}
	n := 0
	for _, c := range state.Crates {
		if goals[c] {
			n++
		}
	}
	return n
}
