package world

import (
	"errors"
	"fmt"

	gameruntime "github.com/maestroi/pokepilot/game"
)

// RoutePrerequisites overlays semantic transition requirements onto the
// geometric map graph. Edge remains the current Red-era graph identity; the
// transition value is the portable contract consumed by the routing policy.
type RoutePrerequisites struct {
	Transitions  map[Edge]gameruntime.Transition
	Capabilities gameruntime.CapabilitySet
}

// RouteBlockedError reports that a geometric route exists, but the usable
// route does not because one or more semantic prerequisites are missing. It
// unwraps to ErrNoRoute so existing callers keep their conservative behavior
// while structured callers can inspect Blockages with errors.As.
type RouteBlockedError struct {
	Blockages []gameruntime.TransitionBlockage
}

func (e *RouteBlockedError) Error() string {
	if e == nil || len(e.Blockages) == 0 {
		return ErrNoRoute.Error()
	}
	return fmt.Sprintf("%s: %s", ErrNoRoute, e.Blockages[0].Error())
}

func (e *RouteBlockedError) Unwrap() error { return ErrNoRoute }

// MissingCapabilities returns a stable de-duplicated list of prerequisites
// observed on the blocked geometric route.
func (e *RouteBlockedError) MissingCapabilities() []gameruntime.CapabilityID {
	if e == nil {
		return nil
	}
	seen := map[gameruntime.CapabilityID]bool{}
	var out []gameruntime.CapabilityID
	for _, blockage := range e.Blockages {
		for _, id := range blockage.Missing {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// FindRouteAtDestinationWithCapabilities preserves FindRouteAtDestination's
// component-aware routing exactly, while removing edges whose semantic
// prerequisites are not usable now. If that removal is the reason routing
// fails, the shortest geometric route is inspected and returned as structured
// prerequisite evidence instead of flattening the result to only ErrNoRoute.
func FindRouteAtDestinationWithCapabilities(
	g *Graph,
	from, to uint8,
	x, y, tx, ty int,
	blockedHere map[Edge]bool,
	prereqs RoutePrerequisites,
) ([]Edge, error) {
	if g == nil || len(prereqs.Transitions) == 0 {
		return FindRouteAtDestination(g, from, to, x, y, tx, ty, blockedHere)
	}

	denied := make(map[Edge]gameruntime.TransitionBlockage)
	for edge, transition := range prereqs.Transitions {
		if blockage, ok := gameruntime.EvaluateTransition(transition, prereqs.Capabilities); !ok {
			denied[edge] = blockage
		}
	}
	if len(denied) == 0 {
		return FindRouteAtDestination(g, from, to, x, y, tx, ty, blockedHere)
	}

	usable := graphWithoutSemanticEdges(g, denied)
	route, err := FindRouteAtDestination(usable, from, to, x, y, tx, ty, blockedHere)
	if err == nil || !errors.Is(err, ErrNoRoute) {
		return route, err
	}

	// Only claim a semantic blockage when the same component-aware geometric
	// planner can actually produce a route before semantic gates are applied.
	geometric, geometricErr := FindRouteAtDestination(g, from, to, x, y, tx, ty, blockedHere)
	if geometricErr != nil {
		return nil, err
	}
	var blockages []gameruntime.TransitionBlockage
	for _, edge := range geometric {
		if blockage, ok := denied[edge]; ok {
			blockages = append(blockages, blockage)
		}
	}
	if len(blockages) == 0 {
		return nil, err
	}
	return nil, &RouteBlockedError{Blockages: blockages}
}

// graphWithoutSemanticEdges is a read-only graph view. Component labels and
// edge port metadata are immutable routing evidence and can be shared; only
// the adjacency slices are copied and filtered.
func graphWithoutSemanticEdges(g *Graph, denied map[Edge]gameruntime.TransitionBlockage) *Graph {
	copyGraph := *g
	copyGraph.Edges = make(map[uint8][]Edge, len(g.Edges))
	for mapID, edges := range g.Edges {
		filtered := make([]Edge, 0, len(edges))
		for _, edge := range edges {
			if _, blocked := denied[edge]; blocked {
				continue
			}
			filtered = append(filtered, edge)
		}
		copyGraph.Edges[mapID] = filtered
	}
	return &copyGraph
}
