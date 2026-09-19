package skill

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func setEventFlag(mem *state.Mem, e state.Event) {
	off := sym.EventFlags + uint16(e)/8
	(*mem)[off] |= 1 << (uint16(e) % 8)
}

func TestSatisfiedRoute12SnorlaxDropsActionPivot(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeConnection, From: route13Map, To: route12Map}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok || transition.ID != "red:route12_snorlax" {
		t.Fatalf("Route 13 -> Route 12 transition = %+v ok=%v, want red:route12_snorlax", transition, ok)
	}

	mem := new(state.Mem)
	if redRouteTransitionEffectComplete(mem, transition) {
		t.Fatal("uncleared Snorlax must keep the action pivot")
	}

	setEventFlag(mem, eventBeatRoute12Snorlax)
	if !redRouteTransitionEffectComplete(mem, transition) {
		t.Fatal("cleared Snorlax must drop the action pivot so ordinary port reachability applies")
	}

	// The compound Route 16 east gate still needs its bicycle annotation after
	// Snorlax is gone; only the flute-only clear actions are omitted.
	bike := gameruntime.Transition{ID: "red:route16_snorlax_bicycle"}
	if redRouteTransitionEffectComplete(mem, bike) {
		t.Fatal("route16_snorlax_bicycle must remain annotated after Snorlax is cleared")
	}

	gate := gameruntime.Transition{ID: "red:route12_snorlax_access", Gate: true}
	if redRouteTransitionEffectComplete(mem, gate) {
		t.Fatal("gates must stay annotated even when related story flags are set")
	}
}
