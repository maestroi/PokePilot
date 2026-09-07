package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// RoutePlanner answers GoTo's first question — "is there a route from where
// the player stands to there?" — without walking a step. It is GoTo's own
// planning pass (BuildGraph, overlay the current map's live WRAM geometry,
// FindRouteAtDestination) hoisted so one graph build can answer for many
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
// Snapshot semantics: the planner is built at one instant, from one position.
// Use it to filter a menu offered from that instant; do not hold it across
// movement.
type RoutePlanner struct {
	graph *world.Graph
	cur   uint8
	x, y  uint8
}

// NewRoutePlanner captures the route geometry as it stands right now: the
// static graph with the current map's live block geometry overlaid, plus the
// player's tile, which is what makes the answer component-aware.
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
	return &RoutePlanner{graph: routeGraph, cur: cur, x: x, y: y}, nil
}

// CanReach reports whether GoTo could plan a route to dest from the position
// this planner was built at. It answers the ROUTE question only — which maps
// connect, from this component — not whether the last leg's tile walk will
// succeed, whether an NPC stands in the corridor, or whether a scripted gate
// will refuse. Those are discovered by walking.
func (p *RoutePlanner) CanReach(dest Destination) bool {
	if p == nil {
		return true // no planner is no evidence; never hide a place on a guess
	}
	_, err := world.FindRouteAtDestination(
		p.graph, p.cur, dest.Map, int(p.x), int(p.y), int(dest.X), int(dest.Y), nil,
	)
	return err == nil
}
