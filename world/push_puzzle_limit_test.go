package world

import (
	"errors"
	"testing"
)

func TestPlanPushPuzzleReportsBoundedStateLimit(t *testing.T) {
	g := rectangularPushGrid(7, 5)
	_, err := PlanPushPuzzle(PushPuzzle{
		Grid:      g,
		Player:    Point{X: 1, Y: 2},
		Movables:  []Movable{{ID: 7, Pos: Point{X: 3, Y: 2}}},
		Goal:      PushGoal{Targets: []Point{{X: 5, Y: 2}}},
		MaxStates: 1,
	})
	if !errors.Is(err, ErrPushPuzzleStateLimit) {
		t.Fatalf("error = %v, want ErrPushPuzzleStateLimit", err)
	}
	var bounded *PushPuzzleError
	if !errors.As(err, &bounded) {
		t.Fatalf("error type = %T, want *PushPuzzleError", err)
	}
	if bounded.Explored != 1 || bounded.MaxStates != 1 {
		t.Fatalf("bounded diagnostic = %+v, want explored=1 maxStates=1", bounded)
	}
	if bounded.Player != (Point{X: 1, Y: 2}) || len(bounded.Movables) != 1 || bounded.Movables[0].ID != 7 {
		t.Fatalf("bounded diagnostic lost observed start state: %+v", bounded)
	}
}
