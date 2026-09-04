package skill

import "testing"

func TestGrindPairPrefersAdjacentGrass(t *testing.T) {
	// The farther vertical strip is denser and won under the old scorer,
	// producing multi-tile pacing. The adjacent cell must now win before any
	// longer-path scoring is considered. grid is intentionally nil: an
	// adjacent pair needs no intermediate collision checks.
	grass := []cell{
		{0, 0},
		{1, 0},
		{0, 2},
		{0, 3},
		{0, 4},
	}
	a, b, ok := grindPair(grass, nil, 0, 0)
	if !ok {
		t.Fatal("grindPair returned no pair")
	}
	if a != (cell{0, 0}) {
		t.Fatalf("a = %+v, want nearest grass cell (0,0)", a)
	}
	if dist(a, b) != 1 {
		t.Fatalf("pair = %+v -> %+v, distance %d; want one-tile ping-pong", a, b, dist(a, b))
	}
}
