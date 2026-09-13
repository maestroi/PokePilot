package world

import "github.com/maestroi/pokepilot/red/rom"

// ConnectionBand returns the inclusive source-border index range carried by a
// component-scoped connection edge. North/south bands are X coordinates;
// west/east bands are Y coordinates. Unscoped/hand-built connection edges
// return ok=false and retain the historical whole-border behavior.
//
// Edge already has two bytes that are meaningful only for warps. Connection
// edges encode start+1/end+1 in WarpX/WarpY, keeping Edge comparable (and all
// existing map[Edge] routing/ban tables intact) without growing the generic
// transition identity for one game's border detail. Zero remains the legacy
// "whole border" representation used by hand-built tests and callers.
func ConnectionBand(e Edge) (start, end int, ok bool) {
	if e.Kind != EdgeConnection || e.WarpX == 0 || e.WarpY == 0 {
		return 0, 0, false
	}
	start, end = int(e.WarpX)-1, int(e.WarpY)-1
	if start < 0 || end < start {
		return 0, 0, false
	}
	return start, end, true
}

func encodeConnectionBand(e Edge, start, end int) (Edge, bool) {
	// The +1 encoding reserves zero for an unscoped edge. Red's maps are far
	// smaller than 255 tiles along a seam; fail closed to the aggregate edge if
	// a future adapter ever exceeds what this compact identity can represent.
	if start < 0 || end < start || end >= 255 {
		return Edge{}, false
	}
	e.WarpX = uint8(start + 1)
	e.WarpY = uint8(end + 1)
	return e, true
}

func connectionBandRange(e Edge, n int) (start, end int) {
	if n <= 0 {
		return 0, -1
	}
	start, end, ok := ConnectionBand(e)
	if !ok {
		return 0, n - 1
	}
	if start >= n {
		return n, n - 1
	}
	if end >= n {
		end = n - 1
	}
	return start, end
}

type connectionComponentPair struct {
	exit  int
	entry int
}

// connectionEdges splits one ROM border connection into contiguous source
// bands whose seam tiles share the same source and destination walkable
// components. The pair matters, not only the destination component: tile-pair
// collision can split adjacent source-edge tiles even when the far side is one
// plaza, and routing must not aggregate those exits back together.
func (g *Graph) connectionEdges(from uint8, c rom.Connection) []Edge {
	base := Edge{Kind: EdgeConnection, From: from, To: c.MapID, Dir: c.Dir}
	src, okSrc := g.tiles[from]
	_, okDst := g.tiles[c.MapID]
	if !okSrc || !okDst || g.comps[from] == nil || g.comps[c.MapID] == nil {
		g.connections[base] = c
		return []Edge{base}
	}

	n := src.w
	if c.Dir >= dirWest {
		n = src.h
	}
	if n <= 0 {
		g.connections[base] = c
		return []Edge{base}
	}

	type run struct {
		start int
		end   int
		pair  connectionComponentPair
	}
	var runs []run
	runStart := -1
	var runPair connectionComponentPair
	flush := func(end int) {
		if runStart < 0 {
			return
		}
		runs = append(runs, run{start: runStart, end: end, pair: runPair})
		runStart = -1
	}

	for i := 0; i < n; i++ {
		sx, sy, tx, ty := g.connectionSeamTile(base, c, i)
		a := standingComponentAt(g, from, sx, sy)
		b := standingComponentAt(g, c.MapID, tx, ty)
		if len(a) == 0 || len(b) == 0 {
			flush(i - 1)
			continue
		}
		pair := connectionComponentPair{exit: a[0], entry: b[0]}
		if runStart < 0 {
			runStart, runPair = i, pair
			continue
		}
		if pair != runPair {
			flush(i - 1)
			runStart, runPair = i, pair
		}
	}
	flush(n - 1)

	if len(runs) == 0 {
		// Preserve the historical edge rather than deleting topology merely
		// because static component evidence is incomplete. canExit will still
		// reject a known-unwalkable port once its component sets are computed.
		g.connections[base] = c
		return []Edge{base}
	}

	out := make([]Edge, 0, len(runs))
	for _, r := range runs {
		e, ok := encodeConnectionBand(base, r.start, r.end)
		if !ok {
			g.connections[base] = c
			return []Edge{base}
		}
		g.connections[e] = c
		out = append(out, e)
	}
	return out
}

// connectionSeamTile maps source-border index i to the standing tiles on both
// sides using the same ROM Offset rule as connectionPortComps.
func (g *Graph) connectionSeamTile(e Edge, c rom.Connection, i int) (sx, sy, tx, ty int) {
	dst := g.tiles[e.To]
	j := i + int(c.Offset)
	sx, sy, tx, ty = i, 0, j, dst.h-1
	switch e.Dir {
	case dirSouth:
		sy, ty = g.tiles[e.From].h-1, 0
	case dirWest:
		sx, sy, tx, ty = 0, i, dst.w-1, j
	case dirEast:
		sx, sy, tx, ty = g.tiles[e.From].w-1, i, 0, j
	}
	return sx, sy, tx, ty
}
