package world

import (
	"fmt"

	"github.com/maestroi/pokepilot/worldmodel"
)

// WithMapGrid returns a routing snapshot whose component data for mapID is
// rebuilt from grid. The base graph is never mutated: live geometry belongs to
// one loaded map instance and must not leak into later map loads.
//
// Connection bands touching mapID are rebuilt from the live component pairs.
// A band is not immutable ROM data: its start/end segmentation is derived from
// component ids on both sides of the seam, so retaining the base bands after a
// live topology change can make the router approve one component while the
// executor crosses a different tile in that stale aggregate band.
func (g *Graph) WithMapGrid(mapID uint8, grid *Grid) (*Graph, error) {
	if g == nil {
		return nil, fmt.Errorf("world: live graph overlay on nil graph")
	}
	if grid == nil {
		return nil, fmt.Errorf("world: live graph overlay for map %02x has nil grid", mapID)
	}
	if grid.MapID != mapID {
		return nil, fmt.Errorf("world: live graph overlay map %02x got grid for map %02x", mapID, grid.MapID)
	}
	if !g.componentAware {
		return g, nil
	}

	out := *g
	out.reachable = make(map[uint8]map[int][]int, len(g.reachable))
	for id, reachable := range g.reachable {
		out.reachable[id] = reachable
	}
	out.comps = make(map[uint8][][]int, len(g.comps))
	for id, comps := range g.comps {
		out.comps[id] = comps
	}
	out.tiles = make(map[uint8]dim, len(g.tiles))
	for id, d := range g.tiles {
		out.tiles[id] = d
	}

	out.comps[mapID] = componentsWithBlocked(grid, warpTileBlockers(g.warps[mapID]))
	out.reachable[mapID] = componentReachability(grid, out.comps[mapID])
	out.tiles[mapID] = dim{w: grid.Width, h: grid.Height}

	out.resegmentConnectionsTouching(g, mapID)

	// Edge identity can change when a connection is split/merged into new
	// bands, so rebuild both port-component caches from the resulting edge set
	// instead of retaining entries keyed by stale edges.
	out.exitComps = make(map[Edge][]int)
	out.entryComps = make(map[Edge][]int)
	for _, edges := range out.Edges {
		for _, e := range edges {
			out.exitComps[e] = out.exitPortComps(e)
			out.entryComps[e] = out.expandComponents(e.To, out.entryPortComps(e))
		}
	}
	return &out, nil
}

type liveConnectionKey struct {
	from, to, dir uint8
	offset        int8
}

// resegmentConnectionsTouching rebuilds every logical connection whose source
// or destination is mapID. connectionEdges derives its bands from component
// pairs on BOTH maps, so changing either side invalidates the old segmentation.
//
// base supplies the immutable pre-overlay edge ordering and connection metadata;
// g already contains the live component matrix for mapID. Rebuilt bands are
// inserted at the first old edge for that logical connection, preserving the
// route graph's relative edge order while dropping any remaining stale bands.
func (g *Graph) resegmentConnectionsTouching(base *Graph, mapID uint8) {
	affected := make(map[Edge]bool)
	for _, edges := range base.Edges {
		for _, e := range edges {
			_, known := base.connections[e]
			if e.Kind == EdgeConnection && known && (e.From == mapID || e.To == mapID) {
				affected[e] = true
			}
		}
	}

	g.Edges = make(map[uint8][]Edge, len(base.Edges))
	g.connections = make(map[Edge]worldmodel.Connection, len(base.connections))
	for e, c := range base.connections {
		if !affected[e] {
			g.connections[e] = c
		}
	}

	seen := make(map[liveConnectionKey]bool)
	for from, edges := range base.Edges {
		rebuilt := make([]Edge, 0, len(edges))
		for _, e := range edges {
			c, known := base.connections[e]
			if !affected[e] || !known {
				rebuilt = append(rebuilt, e)
				continue
			}

			key := liveConnectionKey{from: e.From, to: e.To, dir: e.Dir, offset: c.Offset}
			if seen[key] {
				continue
			}
			seen[key] = true
			rebuilt = append(rebuilt, g.connectionEdges(e.From, c)...)
		}
		g.Edges[from] = rebuilt
	}
}
