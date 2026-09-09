package world

import (
	"errors"
	"fmt"

	gameruntime "github.com/maestroi/pokepilot/game"
)

// RoutePrerequisites overlays semantic transition requirements onto the
// geometric map graph. Edge remains the current graph identity; the transition
// value is the portable contract consumed by both routing and execution.
type RoutePrerequisites struct {
	Transitions  map[Edge]gameruntime.Transition
	Capabilities gameruntime.CapabilitySet
}

// RouteStep preserves the semantic transition identity selected for an edge.
// Transition is nil for ordinary walking/warp edges.
type RouteStep struct {
	Edge       Edge
	Transition *gameruntime.Transition
}

// TransitionExecutionResult is the portable observation returned by a
// game-adapter executor. Changed means the executor positively observed a world
// or traversal-state change, so the caller must discard the remaining route and
// re-plan from fresh state before doing anything else.
type TransitionExecutionResult struct {
	Changed bool
}

// TransitionExecutor is the game-adapter seam for semantic route actions. The
// generic world layer owns only the contract; badge/HM menus, story battles,
// boulders, and other mechanics remain in the adapter.
type TransitionExecutor interface {
	ExecuteTransition(Edge, gameruntime.Transition) (TransitionExecutionResult, error)
}

var (
	ErrTransitionExecutorUnavailable = errors.New("world: semantic transition executor unavailable")
	ErrTransitionExecutionStalled    = errors.New("world: semantic transition execution made no durable progress")
)

// TransitionExecutionError preserves the selected transition/edge and the
// typed adapter cause so objective recovery (#145) can classify it without
// parsing prose.
type TransitionExecutionError struct {
	Edge       Edge
	Transition gameruntime.Transition
	Cause      error
}

func (e *TransitionExecutionError) Error() string {
	if e == nil {
		return "world: semantic transition execution failed"
	}
	return fmt.Sprintf("world: execute transition %q on %02x->%02x: %v", e.Transition.ID, e.Edge.From, e.Edge.To, e.Cause)
}

func (e *TransitionExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ExecuteTransition invokes the adapter through a typed boundary and turns a
// missing executor or adapter failure into structured transition evidence.
func ExecuteTransition(executor TransitionExecutor, edge Edge, transition gameruntime.Transition) (TransitionExecutionResult, error) {
	if executor == nil {
		return TransitionExecutionResult{}, &TransitionExecutionError{
			Edge: edge, Transition: transition, Cause: ErrTransitionExecutorUnavailable,
		}
	}
	result, err := executor.ExecuteTransition(edge, transition)
	if err != nil {
		return TransitionExecutionResult{}, &TransitionExecutionError{Edge: edge, Transition: transition, Cause: err}
	}
	return result, nil
}

// RouteBlockedError reports that a semantic route exists, but one or more
// transitions on it are unusable because capabilities are absent. It unwraps
// to ErrNoRoute so conservative callers keep their existing behavior.
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

// FindRouteAtDestinationWithCapabilities is the compatibility edge-only view
// of FindRoutePlanAtDestinationWithCapabilities.
func FindRouteAtDestinationWithCapabilities(
	g *Graph,
	from, to uint8,
	x, y, tx, ty int,
	blockedHere map[Edge]bool,
	prereqs RoutePrerequisites,
) ([]Edge, error) {
	plan, err := FindRoutePlanAtDestinationWithCapabilities(g, from, to, x, y, tx, ty, blockedHere, prereqs)
	if err != nil {
		return nil, err
	}
	edges := make([]Edge, len(plan))
	for i := range plan {
		edges[i] = plan[i].Edge
	}
	return edges, nil
}

// FindRoutePlanAtDestinationWithCapabilities applies the same semantic policy
// used by reachability filtering and preserves transition identity for
// execution. Capability-satisfied semantic edges are executable pivots: the
// pre-action ordinary-walking component does not have to reach the port. A
// missing capability removes that edge and, when it is the reason routing
// fails, returns structured prerequisite evidence before any movement occurs.
func FindRoutePlanAtDestinationWithCapabilities(
	g *Graph,
	from, to uint8,
	x, y, tx, ty int,
	blockedHere map[Edge]bool,
	prereqs RoutePrerequisites,
) ([]RouteStep, error) {
	if g == nil || len(prereqs.Transitions) == 0 {
		route, err := FindRouteAtDestination(g, from, to, x, y, tx, ty, blockedHere)
		return routeSteps(route, nil), err
	}

	denied := make(map[Edge]gameruntime.TransitionBlockage)
	allowed := make(map[Edge]bool)
	allSemantic := make(map[Edge]bool, len(prereqs.Transitions))
	for edge, transition := range prereqs.Transitions {
		// A gate is a precondition on ordinary geometry, not an action that
		// creates traversal, so it is never a pivot: satisfied or not, the
		// component rules below still decide whether its port is reachable.
		if !transition.Gate {
			allSemantic[edge] = true
		}
		if blockage, ok := gameruntime.EvaluateTransition(transition, prereqs.Capabilities); !ok {
			denied[edge] = blockage
		} else if !transition.Gate {
			allowed[edge] = true
		}
	}

	usable := g
	if len(denied) > 0 {
		usable = graphWithoutSemanticEdges(g, denied)
	}
	route, err := findRouteAtDestinationAllowingSemantic(usable, from, to, x, y, tx, ty, blockedHere, allowed)
	if err == nil {
		return routeSteps(route, prereqs.Transitions), nil
	}
	if !errors.Is(err, ErrNoRoute) || len(denied) == 0 {
		return nil, err
	}

	// Diagnose against the route that would exist if every known semantic
	// action were usable. This is essential for gates such as Surf/Cut whose
	// pre-action walking topology deliberately cannot reach the port.
	geometric, geometricErr := findRouteAtDestinationAllowingSemantic(g, from, to, x, y, tx, ty, blockedHere, allSemantic)
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

func routeSteps(route []Edge, transitions map[Edge]gameruntime.Transition) []RouteStep {
	steps := make([]RouteStep, len(route))
	for i, edge := range route {
		steps[i].Edge = edge
		if transition, ok := transitions[edge]; ok {
			t := transition
			steps[i].Transition = &t
		}
	}
	return steps
}

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
