package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

// TestCeruleanCaveEntranceRequiresHallOfFame pins run-d6dokr184ky81, which
// retried Mewtwo on "no route" for days: the cave guard stands on the
// entrance until the Hall of Fame script hides him, so the Cerulean City ->
// Cerulean Cave 1F warp is a gate on main-story completion, and leaving the
// cave stays ordinary.
func TestCeruleanCaveEntranceRequiresHallOfFame(t *testing.T) {
	enter := world.Edge{Kind: world.EdgeWarp, From: semanticCeruleanCityMap, To: ceruleanCave1FMap, WarpX: 4, WarpY: 11}
	transition, ok := redRouteTransitionForEdge(enter)
	if !ok || !transition.Gate || transition.ID != "red:cerulean_cave_guard" {
		t.Fatalf("cave entrance transition = %+v (ok=%v), want red:cerulean_cave_guard gate", transition, ok)
	}
	leave := world.Edge{Kind: world.EdgeWarp, From: ceruleanCave1FMap, To: semanticCeruleanCityMap}
	if transition, ok := redRouteTransitionForEdge(leave); ok && transition.ID == "red:cerulean_cave_guard" {
		t.Fatal("leaving Cerulean Cave is gated")
	}

	mem := new(state.Mem)
	if redRouteCapabilities(nil, mem).Has(capCanEnterCeruleanCave) {
		t.Fatalf("pre-Hall-of-Fame save projects %s", capCanEnterCeruleanCave)
	}
	mem[0xd734] |= 1 // wElite4Flags: Hall of Fame completed
	if !redRouteCapabilities(nil, mem).Has(capCanEnterCeruleanCave) {
		t.Fatalf("Hall-of-Fame save lacks %s", capCanEnterCeruleanCave)
	}
}
