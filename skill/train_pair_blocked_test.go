package skill

import "testing"

// The Route 1 shape that killed 90 runs: the player stands on grass at
// (14,14) surrounded by grass, and a wandering NPC is parked on (14,13).
// grass is row-major, so (14,13) is the first adjacent cell the fast path
// meets — and GoTo refuses a destination a sprite occupies, so the hunt died
// on leg 1 every single time. The pair must avoid the occupied tile while
// still preferring a one-tile ping-pong.
func TestGrindPairAvoidsSprites(t *testing.T) {
	grass := []cell{
		{14, 13}, {15, 13},
		{14, 14}, {15, 14},
		{14, 15}, {15, 15},
	}
	blocked := map[[2]int]bool{{14, 13}: true}

	a, b, ok := grindPair(grass, nil, 14, 14, blocked)
	if !ok {
		t.Fatal("grindPair returned no pair; free adjacent grass exists")
	}
	if a == (cell{14, 13}) || b == (cell{14, 13}) {
		t.Fatalf("pair %+v -> %+v uses the sprite-occupied cell (14,13)", a, b)
	}
	if a != (cell{14, 14}) {
		t.Fatalf("a = %+v, want the player's own free grass cell (14,14)", a)
	}
	if dist(a, b) != 1 {
		t.Fatalf("pair %+v -> %+v, distance %d; want one-tile ping-pong", a, b, dist(a, b))
	}
}

// Every grass cell occupied leaves no pair to walk, and that must be reported
// rather than returning a doomed destination.
func TestGrindPairAllBlocked(t *testing.T) {
	grass := []cell{{1, 1}, {1, 2}}
	blocked := map[[2]int]bool{{1, 1}: true, {1, 2}: true}
	if _, _, ok := grindPair(grass, nil, 1, 1, blocked); ok {
		t.Fatal("grindPair returned a pair with every grass cell occupied")
	}
}
