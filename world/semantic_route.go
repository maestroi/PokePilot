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
// execution.
//
// PortBypass and ordinary FROM-side actions (Cut a tree on this map, Surf off
// a shore) are executable pivots: the pre-action ordinary-walking component
// does not have to reach the port. PivotOnly annotations alone are narrower —
// identify a destination-side topology change (the obstacle lives on the
// adjacent map), so FROM-side canExit still applies and planning stops at that
// action until live topology is refreshed. PortBypass+PivotOnly is
// the FROM-side bridge that skips canExit without discarding the far map's
// landing (Route 9 Cut toward Route 10). Missing capabilities normally remove
// a semantic edge and, when that is the reason routing fails, return structured
// prerequisite evidence before any movement occurs.
//
// PivotOnly transitions are also the exception for missing capabilities: they
// annotate an ordinary edge whose capability is needed only to bypass static
// component reachability. When that capability is absent, the edge remains
// usable through ordinary geometry if the player can already reach its port;
// the transition is omitted from the returned RouteStep so execution does not
// demand an action that was not needed.
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
	hardDenied := make(map[Edge]gameruntime.TransitionBlockage)
	skipCanExit := make(map[Edge]bool)
	relaxLanding := make(map[Edge]bool)
	executable := make(map[Edge]gameruntime.Transition, len(prereqs.Transitions))
	allSkip := make(map[Edge]bool, len(prereqs.Transitions))
	allRelax := make(map[Edge]bool, len(prereqs.Transitions))
	classifyPrivileges := func(edge Edge, transition gameruntime.Transition, skip, relax map[Edge]bool) {
		switch {
		case transition.Gate:
		case transition.PortBypass && transition.PivotOnly:
			skip[edge] = true
		case transition.PortBypass:
			skip[edge] = true
			relax[edge] = true
		case transition.PivotOnly:
			relax[edge] = true
		default:
			skip[edge] = true
			// See the matching comment in weighted_route.go's classify: a
			// plain gated EdgeWarp's destination is a statically known ROM
			// map, not a live-topology unknown, so it must not be granted
			// boundary/relaxLanding status the way a Surf shore or a
			// straddling Cut tree (EdgeConnection) legitimately is.
			if edge.Kind == EdgeConnection {
				relax[edge] = true
			}
		}
	}
	for edge, transition := range prereqs.Transitions {
		// A gate is a precondition on ordinary geometry, not an action that
		// creates traversal, so it is never a pivot: satisfied or not, the
		// component rules below still decide whether its port is reachable.
		if !transition.Gate {
			classifyPrivileges(edge, transition, allSkip, allRelax)
		}
		if blockage, ok := gameruntime.EvaluateTransition(transition, prereqs.Capabilities); !ok {
			denied[edge] = blockage
			// A normal action or gate cannot be used at all while its
			// prerequisite is missing. PivotOnly is different: without its
			// capability the underlying edge is still ordinary geometry, so
			// keep it and simply withhold the pivot privilege below.
			if !transition.PivotOnly {
				hardDenied[edge] = blockage
			}
			continue
		}

		executable[edge] = transition
		classifyPrivileges(edge, transition, skipCanExit, relaxLanding)
	}

	usable := g
	if len(hardDenied) > 0 {
		usable = graphWithoutSemanticEdges(g, hardDenied)
	}
	route, err := findRouteAtDestinationAllowingSemantic(usable, from, to, x, y, tx, ty, blockedHere, skipCanExit, relaxLanding)
	if err == nil {
		return routeSteps(route, executable), nil
	}
	if errors.Is(err, ErrRouteReplanRequired) {
		// A frontier only defers the answer when something past it could lead
		// to the target. A plain map BFS let a capability-less Surf shore look
		// connected, so an unrelated Route 2 Cut tree made the Power Plant
		// "replan required" and travel walked there before failing
		// (run-d6dokr184ky81). If even this over-approximation cannot reach the
		// target, no replan will, and the honest answer is the diagnosis below.
		if mayReachPastFrontier(usable, from, to, x, y, denied, skipCanExit, relaxLanding) {
			return routeSteps(route, executable), err
		}
		err = ErrNoRoute
	}
	if !errors.Is(err, ErrNoRoute) || len(denied) == 0 {
		return nil, err
	}

	// Diagnose against the route that would exist if every known semantic
	// action were usable. This is essential for actions such as Surf/Cut whose
	// pre-action walking topology deliberately cannot reach the port. It also
	// preserves prerequisite evidence for PivotOnly actions when ordinary
	// geometry cannot reach the annotated edge and the missing capability is
	// exactly what would have allowed the component pivot.
	geometric, geometricErr := findRouteAtDestinationAllowingSemantic(g, from, to, x, y, tx, ty, blockedHere, allSkip, allRelax)
	if geometricErr != nil && !errors.Is(geometricErr, ErrRouteReplanRequired) {
		return nil, err
	}
	blockages := deniedOn(geometric, denied)
	if len(blockages) == 0 && geometricErr != nil {
		// A frontier prefix names only the actions before the unknown landing;
		// look past it for the blockage beyond.
		if past, pastErr := routePastFrontier(g, from, to, x, y, tx, ty, blockedHere, allSkip, allRelax); pastErr == nil {
			blockages = deniedOn(past, denied)
		}
	}
	if len(blockages) == 0 {
		return nil, err
	}
	return nil, &RouteBlockedError{Blockages: blockages}
}

// mayReachPastFrontier is a component-blind map search that over-approximates
// any post-action topology, with one exception. A capability-denied PivotOnly
// edge remains in usable as ordinary geometry, so it is crossed only when some
// landing already reached on its map shares a component with its port: without
// the capability, ordinary walking is the only way onto it. Landings count only
// from maps the search has reached, so a destination's own exit warp (the
// Power Plant door onto its Surf island) cannot vouch for the edge into it. A
// map or edge without component evidence is not proof and stays crossable.
func mayReachPastFrontier(g *Graph, from, to uint8, x, y int, denied map[Edge]gameruntime.TransitionBlockage, skipCanExit, relaxLanding map[Edge]bool) bool {
	landings := map[uint8][]int{from: componentSetAt(g, from, x, y)}
	unknown := map[uint8]bool{from: len(landings[from]) == 0}
	for changed := true; changed; {
		changed = false
		for cur := range landings {
			for _, e := range g.Edges[cur] {
				// Same phantom-band rule as findRoute: a connection band with no
				// walkable exit port is not a hop unless an action bridges it.
				if g.componentAware && len(g.exitComps[e]) == 0 && !(skipCanExit[e] && relaxLanding[e]) {
					continue
				}
				if _, ok := denied[e]; ok && g.componentAware && !unknown[cur] &&
					len(g.exitComps[e]) > 0 && !shareComp(g.exitComps[e], landings[cur]) {
					continue
				}
				entry := g.entryComps[e]
				if len(entry) == 0 && !unknown[e.To] {
					unknown[e.To] = true
					changed = true
				}
				if known, seen := landings[e.To]; !seen || !containsComps(known, entry) {
					landings[e.To] = append(known, entry...)
					changed = true
				}
			}
		}
	}
	// Component-blind at the destination: in-map actions (a Cut tree inside
	// Celadon Gym) can reshape the target map without being graph edges.
	_, ok := landings[to]
	return ok
}

func containsComps(set, want []int) bool {
	for _, c := range want {
		found := false
		for _, have := range set {
			if have == c {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func deniedOn(route []Edge, denied map[Edge]gameruntime.TransitionBlockage) []gameruntime.TransitionBlockage {
	var blockages []gameruntime.TransitionBlockage
	for _, edge := range route {
		if blockage, ok := denied[edge]; ok {
			blockages = append(blockages, blockage)
		}
	}
	return blockages
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
