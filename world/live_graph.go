package world

import "fmt"

// WithMapGrid returns a routing snapshot whose component data for mapID is
// rebuilt from grid. The base graph is never mutated: live geometry belongs to
// one loaded map instance and must not leak into later map loads.
//
// Edges remain the immutable ROM transitions. Only component reachability at
// ports touching mapID is refreshed, which is enough for FindRouteAt* to see a
// door opening/closing or another ReplaceTileBlock change immediately.
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
	out.comps = make(map[uint8][][]int, len(g.comps))
	for id, c := range g.comps {
		out.comps[id] = c
	}
	out.exitComps = make(map[Edge][]int, len(g.exitComps))
	for e, c := range g.exitComps {
		out.exitComps[e] = c
	}
	out.entryComps = make(map[Edge][]int, len(g.entryComps))
	for e, c := range g.entryComps {
		out.entryComps[e] = c
	}
	out.tiles = make(map[uint8]dim, len(g.tiles))
	for id, d := range g.tiles {
		out.tiles[id] = d
	}

	out.comps[mapID] = components(grid)
	out.tiles[mapID] = dim{w: grid.Width, h: grid.Height}

	for _, edges := range out.Edges {
		for _, e := range edges {
			if e.From == mapID {
				out.exitComps[e] = out.exitPortComps(e)
			}
			if e.To == mapID {
				out.entryComps[e] = out.entryPortComps(e)
			}
		}
	}
	return &out, nil
}
