package skill

import (
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

// RedRouteTransitionsForValidation returns the static semantic edge catalog
// without reading live RAM. It exists for offline world-model verification:
// capability-state exploration needs to know which geometric edges are gates or
// semantic pivots, but must not manufacture a save state just to obtain those
// declarations.
//
// This is an adapter export, not generic planner policy. Future games should
// expose their own transition catalog and feed the same worldverify contract.
func RedRouteTransitionsForValidation(g *world.Graph) map[world.Edge]gameruntime.Transition {
	transitions := make(map[world.Edge]gameruntime.Transition)
	if g == nil {
		return transitions
	}
	for _, edges := range g.Edges {
		for _, edge := range edges {
			transition, ok := redRouteTransitionForEdge(edge)
			if !ok {
				continue
			}
			// Only actions that explicitly own a map-edge port may attach to a
			// connection band with no pristine walkable seam. Surf does; Cut,
			// Snorlax and other interior actions do not. Keep this identical to
			// the live prerequisite catalog so verification audits what routing
			// can actually select.
			if edge.Kind == world.EdgeConnection && !transition.PortBypass && !g.ConnectionExitWalkable(edge) {
				continue
			}
			transitions[edge] = transition
		}
	}
	return transitions
}
