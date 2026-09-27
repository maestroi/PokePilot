// Package policy chooses semantic Tetris placements without driving emulator input.
package policy

import (
	"errors"
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/tetris"
	"github.com/maestroi/pokepilot/tetris/control"
)

var (
	ErrNotReady    = errors.New("tetris policy: piece input not ready")
	ErrNoPlacement = errors.New("tetris policy: no legal placement")
)

type Objective string

const (
	ObjectiveAuto     Objective = "auto"
	ObjectiveSurvival Objective = "survival"
	ObjectiveLines    Objective = "lines"
	ObjectiveScore    Objective = "score"
)

type Metrics struct {
	AggregateHeight int `json:"aggregate_height"`
	MaxHeight       int `json:"max_height"`
	Holes           int `json:"holes"`
	Bumpiness       int `json:"bumpiness"`
	Wells           int `json:"wells"`
}

type Candidate struct {
	Placement         control.Placement `json:"placement"`
	LandingY          int               `json:"landing_y"`
	LinesCleared      int               `json:"lines_cleared"`
	ExpectedLineScore int               `json:"expected_line_score"`
	Metrics           Metrics           `json:"metrics"`
	ImmediateScore    int               `json:"immediate_score"`
	LookaheadScore    int               `json:"lookahead_score"`
	TotalScore        int               `json:"total_score"`
	InputSteps        int               `json:"input_steps"`
	ResultBoard       tetris.Board      `json:"-"`
}

type Decision struct {
	Objective  Objective    `json:"objective"`
	Piece      tetris.Piece `json:"piece"`
	Candidate  Candidate    `json:"candidate"`
	Considered int          `json:"considered"`
}

// Candidates returns every reachable placement after applying the same
// simulation, objective scoring, and one-piece lookahead used by Choose.
// Callers that add a bounded decision engine must select only from this set;
// legality and board simulation remain deterministic policy responsibilities.
func Candidates(state tetris.State, objective Objective) (Objective, []Candidate, error) {
	if !state.ReadyForPieceInput || state.Active == nil {
		return "", nil, fmt.Errorf("%w: screen=%s paused=%v locking=%v clearing=%v", ErrNotReady, state.Screen, state.Paused, state.Locking, state.Clearing)
	}
	resolved, err := resolveObjective(objective, state.Mode)
	if err != nil {
		return "", nil, err
	}

	candidates := enumerate(
		state.Board,
		state.Active.Piece,
		state.Active.Rotation,
		state.Active.X,
		state.Active.Y,
		state.Level,
		resolved,
	)
	if len(candidates) == 0 {
		return resolved, nil, fmt.Errorf("%w for %s", ErrNoPlacement, state.Active.Piece)
	}

	for i := range candidates {
		if state.Next == nil {
			continue
		}
		future := enumerate(
			candidates[i].ResultBoard,
			state.Next.Piece,
			0,
			4,
			0,
			state.Level,
			resolved,
		)
		if len(future) == 0 {
			// A placement that leaves no legal spawn for the preview piece is
			// effectively a one-piece-delayed top-out.
			candidates[i].LookaheadScore = -100000
		} else {
			best := future[0].ImmediateScore
			for j := 1; j < len(future); j++ {
				if future[j].ImmediateScore > best {
					best = future[j].ImmediateScore
				}
			}
			candidates[i].LookaheadScore = best / 2
		}
		candidates[i].TotalScore = candidates[i].ImmediateScore + candidates[i].LookaheadScore
	}

	return resolved, candidates, nil
}

// BestCandidate applies the deterministic policy tie-breakers to an evaluated
// candidate set. The bool is false only for an empty set.
func BestCandidate(candidates []Candidate) (Candidate, bool) {
	if len(candidates) == 0 {
		return Candidate{}, false
	}
	best := candidates[0]
	for i := 1; i < len(candidates); i++ {
		if better(candidates[i], best) {
			best = candidates[i]
		}
	}
	return best, true
}

// TopCandidates returns at most limit candidates in deterministic policy order.
// A non-positive limit leaves the complete set available. The input is unchanged.
func TopCandidates(candidates []Candidate, limit int) []Candidate {
	if limit <= 0 || limit >= len(candidates) {
		return candidates
	}
	ranked := append([]Candidate(nil), candidates...)
	sort.SliceStable(ranked, func(i, j int) bool { return better(ranked[i], ranked[j]) })
	return ranked[:limit]
}

// Choose selects one deterministic reachable placement for the current piece.
// If a preview piece is available, a one-piece lookahead is included so the
// policy avoids obvious dead-end placements without coupling to the controller.
func Choose(state tetris.State, objective Objective) (Decision, error) {
	resolved, candidates, err := Candidates(state, objective)
	if err != nil {
		return Decision{}, err
	}
	best, ok := BestCandidate(candidates)
	if !ok {
		return Decision{}, fmt.Errorf("%w for %s", ErrNoPlacement, state.Active.Piece)
	}
	return Decision{
		Objective:  resolved,
		Piece:      state.Active.Piece,
		Candidate:  best,
		Considered: len(candidates),
	}, nil
}

// Step chooses and executes exactly one piece placement. Long-running game
// loops belong to the run/runtime integration layer; this helper is deliberately
// one semantic decision plus one verified Phase-3 controller transaction.
func Step(profile game.CartridgeProfile, m control.Machine, objective Objective) (Decision, control.Result, error) {
	state, err := tetris.Observe(profile, m)
	if err != nil {
		return Decision{}, control.Result{}, err
	}
	decision, err := Choose(state, objective)
	if err != nil {
		return Decision{}, control.Result{}, err
	}
	result, err := control.Place(profile, m, decision.Candidate.Placement)
	if err != nil {
		return decision, result, err
	}
	return decision, result, nil
}

func resolveObjective(objective Objective, mode tetris.Mode) (Objective, error) {
	switch objective {
	case ObjectiveAuto:
		switch mode {
		case tetris.ModeA:
			return ObjectiveScore, nil
		case tetris.ModeB:
			return ObjectiveLines, nil
		case tetris.ModeVersus:
			return ObjectiveSurvival, nil
		default:
			return ObjectiveSurvival, nil
		}
	case ObjectiveSurvival, ObjectiveLines, ObjectiveScore:
		return objective, nil
	default:
		return "", fmt.Errorf("tetris policy: unknown objective %q", objective)
	}
}

func enumerate(
	board tetris.Board,
	piece tetris.Piece,
	startRotation uint8,
	startX, startY int,
	level int,
	objective Objective,
) []Candidate {
	var out []Candidate
	for rotation := uint8(0); rotation < 4; rotation++ {
		for column := 0; column < tetris.BoardWidth; column++ {
			if !reachable(board, piece, startRotation, startX, startY, rotation, column) {
				continue
			}
			resultBoard, landingY, lines, topOut, ok := simulate(board, piece, rotation, column, startY)
			if !ok || topOut {
				continue
			}
			metrics := Measure(resultBoard)
			expected := ExpectedLineScore(lines, level)
			immediate := score(objective, metrics, lines, expected)
			out = append(out, Candidate{
				Placement: control.Placement{
					Rotation: rotation,
					Column:   column,
				},
				LandingY:          landingY,
				LinesCleared:      lines,
				ExpectedLineScore: expected,
				Metrics:           metrics,
				ImmediateScore:    immediate,
				TotalScore:        immediate,
				InputSteps:        rotationDistance(startRotation, rotation) + abs(column-startX),
				ResultBoard:       resultBoard,
			})
		}
	}
	return out
}

// Simulate drops a piece vertically from startY at the requested orientation
// and anchor column, then clears completed rows. It does not model lateral
// movement; Choose separately verifies the controller's rotation/shift path.
func Simulate(board tetris.Board, piece tetris.Piece, rotation uint8, column, startY int) (tetris.Board, int, int, bool, bool) {
	return simulate(board, piece, rotation, column, startY)
}

func simulate(board tetris.Board, piece tetris.Piece, rotation uint8, column, startY int) (tetris.Board, int, int, bool, bool) {
	if !fits(board, piece, rotation, column, startY) {
		return board, startY, 0, false, false
	}
	y := startY
	for fits(board, piece, rotation, column, y+1) {
		y++
	}

	cells, ok := tetris.Cells(piece, rotation)
	if !ok {
		return board, y, 0, false, false
	}
	placed := board
	topOut := false
	for _, cell := range cells {
		x := column + cell.X
		row := y + cell.Y
		if row < 0 {
			topOut = true
			continue
		}
		if x < 0 || x >= tetris.BoardWidth || row >= tetris.BoardHeight {
			return board, y, 0, false, false
		}
		placed[row][x] = true
	}

	cleared, lines := clearLines(placed)
	return cleared, y, lines, topOut, true
}

func reachable(board tetris.Board, piece tetris.Piece, startRotation uint8, startX, y int, targetRotation uint8, targetX int) bool {
	if !fits(board, piece, startRotation, startX, y) {
		return false
	}

	rotation := startRotation
	clockwise := int((rotation - targetRotation + 4) % 4)
	counterClockwise := int((targetRotation - rotation + 4) % 4)
	delta := -1
	steps := clockwise
	if counterClockwise < clockwise {
		delta = 1
		steps = counterClockwise
	}
	for i := 0; i < steps; i++ {
		rotation = uint8((int(rotation) + delta + 4) % 4)
		if !fits(board, piece, rotation, startX, y) {
			return false
		}
	}

	x := startX
	dx := 1
	if targetX < x {
		dx = -1
	}
	for x != targetX {
		x += dx
		if !fits(board, piece, targetRotation, x, y) {
			return false
		}
	}
	return true
}

func fits(board tetris.Board, piece tetris.Piece, rotation uint8, column, y int) bool {
	cells, ok := tetris.Cells(piece, rotation)
	if !ok {
		return false
	}
	for _, cell := range cells {
		x := column + cell.X
		row := y + cell.Y
		if x < 0 || x >= tetris.BoardWidth || row >= tetris.BoardHeight {
			return false
		}
		if row >= 0 && board[row][x] {
			return false
		}
	}
	return true
}

func clearLines(board tetris.Board) (tetris.Board, int) {
	var out tetris.Board
	dst := tetris.BoardHeight - 1
	lines := 0
	for src := tetris.BoardHeight - 1; src >= 0; src-- {
		full := true
		for x := 0; x < tetris.BoardWidth; x++ {
			if !board[src][x] {
				full = false
				break
			}
		}
		if full {
			lines++
			continue
		}
		out[dst] = board[src]
		dst--
	}
	return out, lines
}

// Measure extracts board-shape costs used by all objectives.
func Measure(board tetris.Board) Metrics {
	var heights [tetris.BoardWidth]int
	holes := 0
	maxHeight := 0
	aggregate := 0

	for x := 0; x < tetris.BoardWidth; x++ {
		seenBlock := false
		for y := 0; y < tetris.BoardHeight; y++ {
			if board[y][x] {
				if !seenBlock {
					heights[x] = tetris.BoardHeight - y
					seenBlock = true
				}
			} else if seenBlock {
				holes++
			}
		}
		aggregate += heights[x]
		if heights[x] > maxHeight {
			maxHeight = heights[x]
		}
	}

	bumpiness := 0
	for x := 0; x < tetris.BoardWidth-1; x++ {
		bumpiness += abs(heights[x] - heights[x+1])
	}

	wells := 0
	for x := 0; x < tetris.BoardWidth; x++ {
		left := tetris.BoardHeight
		right := tetris.BoardHeight
		if x > 0 {
			left = heights[x-1]
		}
		if x < tetris.BoardWidth-1 {
			right = heights[x+1]
		}
		depth := min(left, right) - heights[x]
		if depth > 0 {
			wells += depth * (depth + 1) / 2
		}
	}

	return Metrics{
		AggregateHeight: aggregate,
		MaxHeight:       maxHeight,
		Holes:           holes,
		Bumpiness:       bumpiness,
		Wells:           wells,
	}
}

// ExpectedLineScore is the game's Type-A line-clear award from the supported
// revision. Soft-drop points are intentionally omitted because they depend on
// the controller's frame path rather than the final board placement.
func ExpectedLineScore(lines, level int) int {
	base := 0
	switch lines {
	case 1:
		base = 40
	case 2:
		base = 100
	case 3:
		base = 300
	case 4:
		base = 1200
	}
	if level < 0 {
		level = 0
	}
	return base * (level + 1)
}

func score(objective Objective, metrics Metrics, lines, expectedLineScore int) int {
	switch objective {
	case ObjectiveLines:
		return lines*2500 -
			metrics.Holes*800 -
			metrics.AggregateHeight*35 -
			metrics.MaxHeight*90 -
			metrics.Bumpiness*30 -
			metrics.Wells*20
	case ObjectiveScore:
		return expectedLineScore*10 +
			lines*200 -
			metrics.Holes*900 -
			metrics.AggregateHeight*30 -
			metrics.MaxHeight*80 -
			metrics.Bumpiness*25 -
			metrics.Wells*15
	default:
		return lines*600 -
			metrics.Holes*1200 -
			metrics.AggregateHeight*45 -
			metrics.MaxHeight*120 -
			metrics.Bumpiness*40 -
			metrics.Wells*30
	}
}

func better(a, b Candidate) bool {
	if a.TotalScore != b.TotalScore {
		return a.TotalScore > b.TotalScore
	}
	if a.Metrics.Holes != b.Metrics.Holes {
		return a.Metrics.Holes < b.Metrics.Holes
	}
	if a.Metrics.MaxHeight != b.Metrics.MaxHeight {
		return a.Metrics.MaxHeight < b.Metrics.MaxHeight
	}
	if a.LinesCleared != b.LinesCleared {
		return a.LinesCleared > b.LinesCleared
	}
	if a.InputSteps != b.InputSteps {
		return a.InputSteps < b.InputSteps
	}
	if a.Placement.Rotation != b.Placement.Rotation {
		return a.Placement.Rotation < b.Placement.Rotation
	}
	return a.Placement.Column < b.Placement.Column
}

func rotationDistance(from, to uint8) int {
	cw := int((from - to + 4) % 4)
	ccw := int((to - from + 4) % 4)
	if ccw < cw {
		return ccw
	}
	return cw
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
