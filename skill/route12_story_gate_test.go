package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestRoute12EarlyEntrancesRequirePokeFlute(t *testing.T) {
	for _, edge := range []world.Edge{
		{Kind: world.EdgeConnection, From: semanticRoute11Map, To: route12Map},
		{Kind: world.EdgeConnection, From: semanticLavenderTownMap, To: route12Map},
	} {
		transition, ok := redRouteTransitionForEdge(edge)
		if !ok {
			t.Fatalf("edge %02x->%02x has no Route 12 story gate", edge.From, edge.To)
		}
		if transition.ID != "red:route12_snorlax_access" || !transition.Gate {
			t.Fatalf("edge %02x->%02x transition=%+v, want prerequisite gate", edge.From, edge.To, transition)
		}
		if len(transition.Requires) != 1 || transition.Requires[0] != capCanClearSnorlax {
			t.Fatalf("edge %02x->%02x requirements=%v, want can_clear_snorlax", edge.From, edge.To, transition.Requires)
		}
	}
}

func TestRoute12PreFluteCheckpointCanEscape(t *testing.T) {
	for _, edge := range []world.Edge{
		{Kind: world.EdgeConnection, From: route12Map, To: semanticRoute11Map},
		{Kind: world.EdgeConnection, From: route12Map, To: semanticLavenderTownMap},
	} {
		if transition, ok := redRouteTransitionForEdge(edge); ok && transition.ID == "red:route12_snorlax_access" {
			t.Fatalf("reverse escape edge %02x->%02x was incorrectly gated: %+v", edge.From, edge.To, transition)
		}
	}
}
