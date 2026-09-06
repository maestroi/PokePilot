package world

import (
	"errors"
	"testing"
)

func testPushGrid(width, height int, walkable ...Point) *Grid {
	g := &Grid{MapID: 1, Width: width, Height: height, walkable: make([]bool, width*height)}
	for _, p := range walkable {
		g.walkable[p.Y*width+p.X] = true
	}
	return g
}

func rectangularPushGrid(width, height int) *Grid {
	var walkable []Point
	for y := 1; y < height-1; y++ {
		for x := 1; x < width-1; x++ {
			walkable = append(walkable, Point{X: x, Y: y})
		}
	}
	return testPushGrid(width, height, walkable...)
}

func TestPlanPushPuzzleFindsShortestLegalPushes(t *testing.T) {
	g := rectangularPushGrid(7, 5)
	plan, err := PlanPushPuzzle(PushPuzzle{
		Grid:     g,
		Player:   Point{X: 1, Y: 2},
		Movables: []Movable{{ID: 7, Pos: Point{X: 3, Y: 2}}},
		Goal:     PushGoal{Targets: []Point{{X: 5, Y: 2}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Pushes) != 2 {
		t.Fatalf("push count = %d, want 2: %+v", len(plan.Pushes), plan)
	}
	want := []Push{
		{MovableID: 7, Stand: Point{X: 2, Y: 2}, From: Point{X: 3, Y: 2}, To: Point{X: 4, Y: 2}, Direction: StepRight},
		{MovableID: 7, Stand: Point{X: 3, Y: 2}, From: Point{X: 4, Y: 2}, To: Point{X: 5, Y: 2}, Direction: StepRight},
	}
	for i := range want {
		got := plan.Pushes[i]
		if got.MovableID != want[i].MovableID || got.Stand != want[i].Stand || got.From != want[i].From || got.To != want[i].To || got.Direction != want[i].Direction {
			t.Fatalf("push %d = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestPlanPushPuzzleResumesFromObservedMidPuzzleState(t *testing.T) {
	g := rectangularPushGrid(7, 5)
	plan, err := PlanPushPuzzle(PushPuzzle{
		Grid:     g,
		Player:   Point{X: 3, Y: 2},
		Movables: []Movable{{ID: 7, Pos: Point{X: 4, Y: 2}}},
		Goal:     PushGoal{Targets: []Point{{X: 5, Y: 2}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Pushes) != 1 || plan.Pushes[0].From != (Point{X: 4, Y: 2}) || plan.Pushes[0].To != (Point{X: 5, Y: 2}) {
		t.Fatalf("resume plan = %+v, want one final push", plan)
	}
}

func TestPlanPushPuzzleCanMakeExitReachable(t *testing.T) {
	g := testPushGrid(6, 5,
		Point{X: 1, Y: 1}, Point{X: 2, Y: 1},
		Point{X: 1, Y: 2}, Point{X: 2, Y: 2}, Point{X: 3, Y: 2}, Point{X: 4, Y: 2},
		Point{X: 2, Y: 3},
	)
	exit := Point{X: 4, Y: 2}
	plan, err := PlanPushPuzzle(PushPuzzle{
		Grid:      g,
		Player:    Point{X: 1, Y: 2},
		Movables:  []Movable{{ID: 1, Pos: Point{X: 2, Y: 2}}},
		Goal:      PushGoal{Reachable: &exit},
		MaxStates: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Pushes) != 1 || plan.Pushes[0].Direction != StepDown {
		t.Fatalf("plan = %+v, want one downward push into the alcove", plan)
	}
	if len(plan.FinalWalk) == 0 {
		t.Fatal("reachable goal returned no final walk")
	}
}

func TestPlanPushPuzzleDetectsStaticCornerDeadlock(t *testing.T) {
	g := rectangularPushGrid(5, 5)
	targets := map[[2]int]bool{{3, 3}: true}
	if !IsStaticPushDeadlock(g, nil, Point{X: 1, Y: 1}, targets) {
		t.Fatal("corner boulder was not identified as a static deadlock")
	}

	_, err := PlanPushPuzzle(PushPuzzle{
		Grid:     g,
		Player:   Point{X: 2, Y: 2},
		Movables: []Movable{{ID: 1, Pos: Point{X: 1, Y: 1}}},
		Goal:     PushGoal{Targets: []Point{{X: 3, Y: 3}}},
	})
	if !errors.Is(err, ErrPushPuzzleNoSolution) {
		t.Fatalf("error = %v, want ErrPushPuzzleNoSolution", err)
	}
}

func TestPlanPushPuzzleAllowsTerminalNonWalkableTarget(t *testing.T) {
	g := testPushGrid(6, 3,
		Point{X: 1, Y: 1}, Point{X: 2, Y: 1}, Point{X: 3, Y: 1},
	)
	plan, err := PlanPushPuzzle(PushPuzzle{
		Grid:     g,
		Player:   Point{X: 1, Y: 1},
		Movables: []Movable{{ID: 2, Pos: Point{X: 3, Y: 1}}},
		Goal:     PushGoal{Targets: []Point{{X: 4, Y: 1}}}, // hole/sink: deliberately not walkable
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Pushes) != 1 || plan.Pushes[0].To != (Point{X: 4, Y: 1}) {
		t.Fatalf("terminal target plan = %+v, want push into (4,1)", plan)
	}
}
