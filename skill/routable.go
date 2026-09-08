package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// RoutePlanner answers GoTo's first question — "is there a route from where
// the player stands to there?" — without walking a step. It is GoTo's own
// planning pass (BuildGraph, overlay the current map's live WRAM geometry,
// component-aware routing) hoisted so one graph build can answer for many
// destinations.
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
}

// NewRoutePlanner captures the route geometry and semantic permissions as they
// stand right now: the static graph with the current map's live block geometry
// overlaid, the player's tile, and Red's adapter projection of route
// capabilities. The world router consumes only the semantic prerequisite
// contract; Red map ids, badges, HMs, and story encoding stay in skill.
func NewRoutePlanner(m *emu.Emu, romData []byte) (*RoutePlanner, error) {
	g, err := world.BuildGraph(romData)
	if err != nil {
		return nil, fmt.Errorf("skill: RoutePlanner: build graph: %w", err)
	}
	cur := m.Peek8(sym.CurMap)
	x, y := playerXY(m)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return nil, fmt.Errorf("skill: RoutePlanner: parse map %02x: %w", cur, err)
	}
	liveGrid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return nil, fmt.Errorf("skill: RoutePlanner: build live map %02x: %w", cur, err)
	}
	routeGraph, err := g.WithMapGrid(cur, liveGrid)
	if err != nil {
		return nil, fmt.Errorf("skill: RoutePlanner: overlay live topology for map %02x: %w", cur, err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	return &RoutePlanner{
		graph:   routeGraph,
		cur:     cur,
		x:       x,
		y:       y,
		prereqs: redRoutePrerequisites(routeGraph, romData, &mem),
	}, nil
}

// Reachability reports the structured route result for dest without moving.
// A *world.RouteBlockedError means geometry exists but a semantic capability
// is missing; plain world.ErrNoRoute means the component-aware graph itself
// cannot reach the target from this position.
func (p *RoutePlanner) Reachability(dest Destination) error {
	if p == nil {
		return nil // no planner is no evidence; never hide a place on a guess
	}
	_, err := world.FindRouteAtDestinationWithCapabilities(
		p.graph, p.cur, dest.Map, int(p.x), int(p.y), int(dest.X), int(dest.Y), nil, p.prereqs,
	)
	return err
}

// CanReach reports whether GoTo could plan a route to dest from the position
// this planner was built at, using capabilities usable in the same snapshot.
// It answers routing/prerequisite facts only; execution still owns dynamic NPCs,
// battles, dialogue, choices, and resource spending encountered while walking.
func (p *RoutePlanner) CanReach(dest Destination) bool {
	return p == nil || p.Reachability(dest) == nil
}
