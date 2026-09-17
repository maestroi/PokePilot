package world

import "github.com/maestroi/pokepilot/red/rom"

// ConnectionBand returns the inclusive source-border index range carried by a
// component-scoped connection edge. North/south bands are X coordinates;
// west/east bands are Y coordinates. Unscoped/hand-built connection edges
// return ok=false and retain the historical whole-border behavior.
func ConnectionBand(e Edge) (start, end int, ok bool) {
	if e.Kind != EdgeConnection || !e.BandScoped {
		return 0, 0, false
	}
	start, end = int(e.BandStart), int(e.BandEnd)
	if end < start {
		return 0, 0, false
	}
	return start, end, true
}

// ConnectionExitWalkable reports whether a concrete connection edge has at
// least one physically standable seam tile on both maps. It only returns false
// when the component-aware graph has enough geometry to prove the band is a
// non-walkable padding band. Missing/incomplete component data remains
// permissive so hand-built graphs and partially decoded maps keep their
// historical behavior.
//
// Semantic route actions normally bypass ordinary canExit reachability because
// actions such as Surf and Cut can create traversal that pristine collision
// does not expose. Adapter code can use this narrower predicate when an action
// annotates a real border crossing but must not claim the non-walkable bands
// retained by connectionEdges solely for semantic routing.
func (g *Graph) ConnectionExitWalkable(e Edge) bool {
	if e.Kind != EdgeConnection {
		return false
	}
	if g == nil || !g.componentAware || g.comps[e.From] == nil || g.comps[e.To] == nil {
		return true
	}
	if _, ok := g.connections[e]; !ok {
		return true
	}
	return len(g.connectionPortComps(e, false)) > 0
}

func encodeConnectionBand(e Edge, start, end int) (Edge, bool) {
	if start < 0 || end < start || end > 255 {
		return Edge{}, false
	}
	e.BandStart = uint8(start)
	e.BandEnd = uint8(end)
	e.BandScoped = true
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
	dst, okDst := g.tiles[c.MapID]
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
		// Offset can leave part of the source edge outside the actual overlap;
		// that is not a physical connection band. A tile that is in-bounds but
		// non-walkable *is* retained with component 0 so semantic transitions
		// such as Surf can still own it while ordinary canExit rejects it.
		if sx < 0 || sy < 0 || sx >= src.w || sy >= src.h ||
			tx < 0 || ty < 0 || tx >= dst.w || ty >= dst.h {
			flush(i - 1)
			continue
		}
		exitComp, entryComp := 0, 0
		if a := standingComponentAt(g, from, sx, sy); len(a) > 0 {
			exitComp = a[0]
		}
		if b := standingComponentAt(g, c.MapID, tx, ty); len(b) > 0 {
			entryComp = b[0]
		}
		pair := connectionComponentPair{exit: exitComp, entry: entryComp}
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
