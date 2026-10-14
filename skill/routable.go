package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
)

// RoutePlanner answers GoTo's first question — "is there a route from where
// the player stands to there?" — without walking a step. It overlays the
// current map's live WRAM geometry on the cached immutable ROM graph, then one
// component-aware routing snapshot can answer for many destinations.
//
// It exists because the menu was offering journeys the router cannot start.
// MEASURED 2026-09-07 on run-3t5kvlk55zvbjkkkno3ses4g6: from Mt. Moon 1F's
// (5,5) ladder landing, "go to viridian city" was picked FOUR times and every
// one died on `world: no route` — the map graph says the maps touch, but the
// component the player stands in reaches no exit that leads there. An
// objective the router refuses before moving is not a choice, it is a wasted
// round, and the planner cannot tell the difference from the menu.
//
// Snapshot semantics: the planner is built at one instant, from one position
// and one capability set. Use it to filter a menu offered from that instant;
// do not hold it across movement, inventory, party, badge, or story changes.
type RoutePlanner struct {
	graph   *world.Graph
	cur     uint8
	x, y    uint8
	prereqs world.RoutePrerequisites
	// prereqsAt re-projects prerequisites for a journey starting elsewhere
	// (a Fly landing); nil means the snapshot's projection is position-free.
	prereqsAt func(from Destination) world.RoutePrerequisites
}

// NewRoutePlanner captures the route geometry and semantic permissions as they
// stand right now: the cached static graph with the current map's live block
// geometry overlaid, the player's tile, and Red's adapter projection of route
// capabilities. The world router consumes only the semantic prerequisite
// contract; Red map ids, badges, HMs, and story encoding stay in skill.
func NewRoutePlanner(m *emu.Emu, romData []byte) (*RoutePlanner, error) {
	g, err := cachedRouteGraph(romData)
	if err != nil {
		return nil, fmt.Errorf("skill: RoutePlanner: build graph: %w", err)
	}
	live, err := currentRoutingRuntime(m)
	if err != nil {
		return nil, fmt.Errorf("skill: RoutePlanner: observe overworld: %w", err)
	}
	cur, x, y := live.Map, live.X, live.Y
	h, err := routingHeaderFor(m, cur)
	if err != nil {
		return nil, fmt.Errorf("skill: RoutePlanner: parse map %02x: %w", cur, err)
	}
	liveGrid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return nil, fmt.Errorf("skill: RoutePlanner: build live map %02x: %w", cur, err)
	}
	// GoTo routes on the live grid WITH present stationary objects (boulders,
	// stay NPCs; see overlayObservedMapTopology). Omitting them here offered
	// "go to victory road 1f" from 2F's exit-side pocket, where a reset boulder
	// seals the 1F ladder, and GoTo refused it every round until the
	// stagnation watchdog fired (run-3ftqlmhp1tmm3cpzdasspkv2o).
	for at := range presentStationaryObjectBlockers(m, h) {
		liveGrid.Set(at[0], at[1], false)
	}
	routeGraph, prereqs, prereqsAt, err := routePlannerGen1Compatibility(m, romData, g, cur, liveGrid)
	if err != nil {
		return nil, fmt.Errorf("skill: RoutePlanner: overlay live topology for map %02x: %w", cur, err)
	}
	return &RoutePlanner{
		graph:     routeGraph,
		cur:       cur,
		x:         x,
		y:         y,
		prereqs:   prereqs,
		prereqsAt: prereqsAt,
	}, nil
}

// prereqsFrom is the semantic prerequisite projection for a journey starting
// at from. Adapter pivots may be scoped to the player's own position, so
// pricing a Fly landing with the live position's projection can reject every
// landing (run-18kg33tqm1mn5c7umo1adri8m).
func (p *RoutePlanner) prereqsFrom(from Destination) world.RoutePrerequisites {
	if p.prereqsAt == nil || (from.Map == p.cur && from.X == p.x && from.Y == p.y) {
		return p.prereqs
	}
	return p.prereqsAt(from)
}

// Reachability reports the structured route result for dest without moving.
// A *world.RouteBlockedError means geometry exists but a semantic capability
// is missing; plain world.ErrNoRoute means the component-aware graph itself
// cannot reach the target from this position.
func (p *RoutePlanner) Reachability(dest Destination) error {
	if p == nil {
		return nil // no planner is no evidence; never hide a place on a guess
	}
	_, err := findRoutePlanForDestination(
		p.graph, p.cur, int(p.x), int(p.y), dest, nil, p.prereqs,
	)
	if errors.Is(err, world.ErrRouteReplanRequired) {
		// The target lies beyond an executable semantic frontier. GoTo can
		// advance to that action and re-plan from refreshed live topology; this
		// is routable for objective offering without claiming a concrete
		// post-action component in the world planner itself.
		return nil
	}
	return err
}

// CanReach reports whether GoTo could plan a route to dest from the position
// this planner was built at, using capabilities usable in the same snapshot.
// It answers routing/prerequisite facts only; execution still owns dynamic NPCs,
// battles, dialogue, choices, and resource spending encountered while walking.
func (p *RoutePlanner) CanReach(dest Destination) bool {
	return p == nil || p.Reachability(dest) == nil
}
