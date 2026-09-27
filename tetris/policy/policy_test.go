package policy

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/tetris"
)

func readyState(piece tetris.Piece) tetris.State {
	return tetris.State{
		Mode:  tetris.ModeA,
		Board: tetris.Board{},
		Active: &tetris.PieceState{
			Piece:    piece,
			Rotation: 0,
			X:        4,
			Y:        0,
		},
		ReadyForPieceInput: true,
	}
}

func TestChooseLinesFillsUniqueBottomGap(t *testing.T) {
	state := readyState(tetris.PieceI)
	state.Mode = tetris.ModeB
	for x := 0; x < tetris.BoardWidth; x++ {
		if x >= 3 && x <= 6 {
			continue
		}
		state.Board[tetris.BoardHeight-1][x] = true
	}

	got, err := Choose(state, ObjectiveLines)
	if err != nil {
		t.Fatal(err)
	}
	if got.Objective != ObjectiveLines {
		t.Fatalf("objective = %s", got.Objective)
	}
	if got.Candidate.Placement.Rotation != 0 || got.Candidate.Placement.Column != 5 {
		t.Fatalf("placement = %#v, want rotation 0 column 5", got.Candidate.Placement)
	}
	if got.Candidate.LinesCleared != 1 {
		t.Fatalf("lines = %d, want 1", got.Candidate.LinesCleared)
	}
	if got.Candidate.Metrics.Holes != 0 {
		t.Fatalf("holes = %d, want 0", got.Candidate.Metrics.Holes)
	}
}

func TestChooseScorePrefersTetris(t *testing.T) {
	state := readyState(tetris.PieceI)
	state.Level = 9
	for y := tetris.BoardHeight - 4; y < tetris.BoardHeight; y++ {
		for x := 0; x < tetris.BoardWidth; x++ {
			if x != 5 {
				state.Board[y][x] = true
			}
		}
	}

	got, err := Choose(state, ObjectiveScore)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate.Placement.Rotation != 1 || got.Candidate.Placement.Column != 6 {
		t.Fatalf("placement = %#v, want vertical I in column 5", got.Candidate.Placement)
	}
	if got.Candidate.LinesCleared != 4 {
		t.Fatalf("lines = %d, want 4", got.Candidate.LinesCleared)
	}
	if got.Candidate.ExpectedLineScore != 12000 {
		t.Fatalf("expected line score = %d, want 12000", got.Candidate.ExpectedLineScore)
	}
}

func TestChooseUsesPreviewLookahead(t *testing.T) {
	state := readyState(tetris.PieceO)
	state.Next = &tetris.PiecePreview{Piece: tetris.PieceI}

	got, err := Choose(state, ObjectiveSurvival)
	if err != nil {
		t.Fatal(err)
	}
	if got.Considered == 0 {
		t.Fatal("expected candidates")
	}
	if got.Candidate.TotalScore != got.Candidate.ImmediateScore+got.Candidate.LookaheadScore {
		t.Fatalf("scores = immediate %d lookahead %d total %d", got.Candidate.ImmediateScore, got.Candidate.LookaheadScore, got.Candidate.TotalScore)
	}
}

func TestChooseRejectsNotReady(t *testing.T) {
	state := readyState(tetris.PieceT)
	state.ReadyForPieceInput = false
	state.Locking = true

	_, err := Choose(state, ObjectiveSurvival)
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("error = %v, want ErrNotReady", err)
	}
}

func TestAutoObjectiveFollowsGameMode(t *testing.T) {
	tests := []struct {
		mode tetris.Mode
		want Objective
	}{
		{tetris.ModeA, ObjectiveScore},
		{tetris.ModeB, ObjectiveLines},
		{tetris.ModeVersus, ObjectiveSurvival},
		{tetris.ModeUnknown, ObjectiveSurvival},
	}
	for _, tc := range tests {
		got, err := resolveObjective(ObjectiveAuto, tc.mode)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("mode %s => %s, want %s", tc.mode, got, tc.want)
		}
	}
}

func TestSimulateClearsLine(t *testing.T) {
	var board tetris.Board
	for x := 0; x < tetris.BoardWidth; x++ {
		if x >= 3 && x <= 6 {
			continue
		}
		board[tetris.BoardHeight-1][x] = true
	}

	got, landingY, lines, topOut, ok := Simulate(board, tetris.PieceI, 0, 5, 0)
	if !ok || topOut {
		t.Fatalf("simulate ok=%v topOut=%v", ok, topOut)
	}
	if landingY != tetris.BoardHeight-1 {
		t.Fatalf("landing Y = %d, want %d", landingY, tetris.BoardHeight-1)
	}
	if lines != 1 {
		t.Fatalf("lines = %d, want 1", lines)
	}
	if got != (tetris.Board{}) {
		t.Fatalf("board not empty after clearing only row: %#v", got)
	}
}

func TestMeasureCountsHeightsHolesAndBumpiness(t *testing.T) {
	var board tetris.Board
	board[15][0] = true
	board[17][0] = true // hole at row 16 below first block
	board[17][1] = true

	got := Measure(board)
	if got.AggregateHeight != 4 {
		t.Fatalf("aggregate height = %d, want 4", got.AggregateHeight)
	}
	if got.MaxHeight != 3 {
		t.Fatalf("max height = %d, want 3", got.MaxHeight)
	}
	if got.Holes != 1 {
		t.Fatalf("holes = %d, want 1", got.Holes)
	}
	if got.Bumpiness == 0 {
		t.Fatal("expected non-zero bumpiness")
	}
}

func TestExpectedLineScoreMatchesGameTable(t *testing.T) {
	tests := []struct {
		lines int
		level int
		want  int
	}{
		{0, 0, 0},
		{1, 0, 40},
		{2, 0, 100},
		{3, 0, 300},
		{4, 0, 1200},
		{4, 9, 12000},
	}
	for _, tc := range tests {
		if got := ExpectedLineScore(tc.lines, tc.level); got != tc.want {
			t.Fatalf("ExpectedLineScore(%d,%d) = %d, want %d", tc.lines, tc.level, got, tc.want)
		}
	}
}

func TestReachableUsesControllerRotationPath(t *testing.T) {
	var board tetris.Board
	// T rotation 1 at spawn anchor (4,0) occupies board cell (3,0).
	// Blocking it means the controller cannot take B from rotation 0 to 1.
	board[0][3] = true

	if reachable(board, tetris.PieceT, 0, 4, 0, 1, 4) {
		t.Fatal("rotation through occupied cell unexpectedly reachable")
	}
}

func TestUnknownObjectiveRejected(t *testing.T) {
	_, err := Choose(readyState(tetris.PieceT), Objective("speed"))
	if err == nil {
		t.Fatal("unknown objective unexpectedly accepted")
	}
}
