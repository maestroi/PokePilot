package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func preparedSemanticEdge(t *testing.T, m *emu.Emu, transitionID string) (world.Edge, gameruntime.Transition) {
	t.Helper()
	g, err := world.BuildGraph(m.ROM())
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	cur := m.Peek8(sym.CurMap)
	for _, edge := range g.Edges[cur] {
		transition, ok := redRouteTransitionForEdge(edge)
		if ok && transition.ID == transitionID {
			return edge, transition
		}
	}
	t.Fatalf("map %02x has no %q semantic edge", cur, transitionID)
	return world.Edge{}, gameruntime.Transition{}
}

// POKEPILOT_CUT_ROUTE_TEST_STATE is a controllable Vermilion City checkpoint
// from which the Gym Cut tree is reachable and Cut is usable/preparable. It
// verifies the semantic executor changes live topology and ordinary Traverse
// can then cross the selected graph edge.
func TestSemanticCutRouteTransitionRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_CUT_ROUTE_TEST_STATE")
	if got := m.Peek8(sym.CurMap); got != semanticVermilionCityMap {
		t.Fatalf("Cut route checkpoint map=%02x, want Vermilion City %02x", got, semanticVermilionCityMap)
	}
	edge, transition := preparedSemanticEdge(t, m, "red:vermilion_gym_cut")
	result, err := world.ExecuteTransition(newRedRouteTransitionExecutor(m, m.ROM(), nil), edge, transition)
	if err != nil {
		t.Fatalf("execute Cut transition: %v", err)
	}
	if !result.Changed {
		t.Fatal("prepared Cut checkpoint did not observe a topology change")
	}
	if err := Traverse(m, m.ROM(), edge); err != nil {
		t.Fatalf("Traverse after Cut: %v", err)
	}
	if got := m.Peek8(sym.CurMap); got != edge.To {
		t.Fatalf("map after Cut transition=%02x, want %02x", got, edge.To)
	}
}

// POKEPILOT_SURF_ROUTE_TEST_STATE is a controllable Pallet Town shoreline
// checkpoint with Surf usable/preparable. The executor must enter verified
// Surf mode and Traverse must continue across the Route 21 connection.
func TestSemanticSurfRouteTransitionRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_SURF_ROUTE_TEST_STATE")
	if got := m.Peek8(sym.CurMap); got != semanticPalletTownMap {
		t.Fatalf("Surf route checkpoint map=%02x, want Pallet Town %02x", got, semanticPalletTownMap)
	}
	edge, transition := preparedSemanticEdge(t, m, "red:route21_surf")
	result, err := world.ExecuteTransition(newRedRouteTransitionExecutor(m, m.ROM(), nil), edge, transition)
	if err != nil {
		t.Fatalf("execute Surf transition: %v", err)
	}
	if !result.Changed || m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
		t.Fatalf("Surf execution=%+v state=%d, want changed + surfing", result, m.Peek8(sym.WalkBikeSurfState))
	}
	if err := Traverse(m, m.ROM(), edge); err != nil {
		t.Fatalf("Traverse while surfing: %v", err)
	}
	if got := m.Peek8(sym.CurMap); got != edge.To {
		t.Fatalf("map after Surf transition=%02x, want %02x", got, edge.To)
	}
}

// POKEPILOT_VIRIDIAN_NORTH_GATE_TEST_STATE is a controllable Viridian City
// checkpoint with the Pokedex already acquired (Oak's parcel delivered), which
// is exactly the farm-observed failure from issue #230: run-38fyc293s9mqhlc9u7o8m22t5
// died at (23,26) trying to cross Route 2 with "no Red executor owns semantic
// transition \"red:viridian_north_pokedex\"" despite the capability already
// being satisfied, because that pure story gate (and its Pewter sibling) had
// no executor case at all and fell into the default "unowned" branch.
func TestViridianNorthGateExecutesOnceCapableRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_VIRIDIAN_NORTH_GATE_TEST_STATE")
	edge, transition := preparedSemanticEdge(t, m, "red:viridian_north_pokedex")
	if _, err := world.ExecuteTransition(newRedRouteTransitionExecutor(m, m.ROM(), nil), edge, transition); err != nil {
		t.Fatalf("execute viridian_north_pokedex gate with Pokedex already acquired: %v", err)
	}
}
