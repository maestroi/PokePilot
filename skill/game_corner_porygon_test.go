package skill

import "testing"

func TestPorygonGameCornerDestinations(t *testing.T) {
	checks := map[string]Destination{
		gameCornerCoinCasePlace:  {Map: 0x8A, X: 0, Y: 2},
		gameCornerCoinClerkPlace: {Map: 0x87, X: 5, Y: 7},
		gameCornerPorygonPlace:   {Map: 0x89, X: 4, Y: 3},
	}
	for name, want := range checks {
		got, ok := Place(name)
		if !ok || got != want {
			t.Fatalf("Place(%q) = %+v, %v; want %+v, true", name, got, ok, want)
		}
	}
}

func TestGameCornerCoinClerkIsSemanticActor(t *testing.T) {
	if !IsGameCornerDexActor(0x87, 5, 6) {
		t.Fatal("Game Corner coin clerk should be owned by semantic Porygon acquisition")
	}
	if IsGameCornerDexActor(0x87, 5, 7) {
		t.Fatal("standing tile must not be classified as the clerk actor")
	}
}

func TestMaxPorygonCoinBudget(t *testing.T) {
	if got := MaxPorygonCoinBudget(); got != 200000 {
		t.Fatalf("MaxPorygonCoinBudget = %d, want 200000", got)
	}
}
