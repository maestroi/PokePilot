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
			// Semantic actions are allowed to bypass pristine component
			// reachability. Route 12's Snorlax action does not own the map seam,
			// so attaching it to a padding-only connection band would turn solid
			// border padding into an executable route. Keep exactly the same
			// adapter constraint as live routing.
			if transition.ID == "red:route12_snorlax" && edge.Kind == world.EdgeConnection && !g.ConnectionExitWalkable(edge) {
				continue
			}
			transitions[edge] = transition
		}
	}
	return transitions
}
