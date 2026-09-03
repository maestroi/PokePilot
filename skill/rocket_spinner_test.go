package skill

import "testing"

func TestRocketSpinnerTransitionTables(t *testing.T) {
	if got := rocketB2FSpins[rocketPoint{4, 9}]; got != (rocketPoint{2, 9}) {
		t.Fatalf("B2F (4,9) lands at %v, want (2,9)", got)
	}
	if got := rocketB2FSpins[rocketPoint{11, 14}]; got != (rocketPoint{15, 18}) {
		t.Fatalf("B2F (11,14) lands at %v, want (15,18)", got)
	}
	if got := rocketB3FSpins[rocketPoint{10, 13}]; got != (rocketPoint{14, 13}) {
		t.Fatalf("B3F (10,13) lands at %v, want (14,13)", got)
	}
	if got := rocketB3FSpins[rocketPoint{15, 18}]; got != (rocketPoint{15, 22}) {
		t.Fatalf("B3F (15,18) lands at %v, want (15,22)", got)
	}
	if len(rocketSpinnerTransitions(rocketHideoutB1FMap)) != 0 {
		t.Fatal("B1F unexpectedly has spinner transitions")
	}
}

func TestPlanRocketSpinnerUsesForcedLanding(t *testing.T) {
	const width, height = 7, 3
	walkableCells := map[rocketPoint]bool{
		{0, 1}: true,
		{1, 1}: true,
		{4, 1}: true,
		{5, 1}: true,
	}
	walkable := func(x, y int) bool { return walkableCells[rocketPoint{x, y}] }
	transitions := map[rocketPoint]rocketPoint{{1, 1}: {4, 1}}

	actions, err := planRocketSpinner(width, height, walkable, 0, 1, 6, 1, transitions, nil)
	if err != nil {
		t.Fatalf("planRocketSpinner: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions = %v, want 2 actions", actions)
	}
	if !actions[0].Forced || actions[0].Enter != (rocketPoint{1, 1}) || actions[0].Landing != (rocketPoint{4, 1}) {
		t.Fatalf("first action = %+v, want forced (1,1)->(4,1)", actions[0])
	}
	if actions[1].Forced || actions[1].Landing != (rocketPoint{5, 1}) {
		t.Fatalf("second action = %+v, want ordinary step to (5,1)", actions[1])
	}
}

func TestPlanRocketSpinnerDoesNotPretendArrowEntryIsOrdinary(t *testing.T) {
	const width, height = 5, 3
	walkableCells := map[rocketPoint]bool{
		{0, 1}: true,
		{1, 1}: true,
		{2, 1}: true,
		{3, 1}: true,
	}
	walkable := func(x, y int) bool { return walkableCells[rocketPoint{x, y}] }
	// Entering (1,1) throws the player back to the start. There is therefore
	// no route to a tile beside warp (4,1), even though a static tile BFS would
	// incorrectly walk straight across the arrow square.
	transitions := map[rocketPoint]rocketPoint{{1, 1}: {0, 1}}

	if _, err := planRocketSpinner(width, height, walkable, 0, 1, 4, 1, transitions, nil); err == nil {
		t.Fatal("spinner planner found a path by treating the forced arrow as an ordinary tile")
	}
}
