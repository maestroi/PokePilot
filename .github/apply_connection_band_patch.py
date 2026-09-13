from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}: {old[:80]!r}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "world/graph.go",
    '''\tWarpX uint8 // EdgeWarp only: the warp tile on the source map\n\tWarpY uint8\n\tDir   uint8 // EdgeConnection only: 0=north 1=south 2=west 3=east\n''',
    '''\tWarpX uint8 // EdgeWarp: tile X; EdgeConnection: encoded band start (see ConnectionBand)\n\tWarpY uint8 // EdgeWarp: tile Y; EdgeConnection: encoded band end\n\tDir   uint8 // EdgeConnection only: 0=north 1=south 2=west 3=east\n''',
)

replace_once(
    "world/graph.go",
    '''\t\tfor _, c := range h.Connections {\n\t\t\tg.Edges[id] = append(g.Edges[id], Edge{\n\t\t\t\tKind: EdgeConnection,\n\t\t\t\tFrom: id,\n\t\t\t\tTo:   c.MapID,\n\t\t\t\tDir:  c.Dir,\n\t\t\t})\n\t\t}\n''',
    '''\t\tfor _, c := range h.Connections {\n\t\t\tg.Edges[id] = append(g.Edges[id], g.connectionEdges(id, c)... )\n\t\t}\n''',
)

replace_once(
    "world/graph.go",
    '''\tvar out []int\n\tseen := map[int]bool{}\n\tfor i := 0; i < n; i++ {\n''',
    '''\tvar out []int\n\tseen := map[int]bool{}\n\tstart, end := connectionBandRange(e, n)\n\tfor i := start; i <= end; i++ {\n''',
)

replace_once(
    "skill/warp.go",
    'tx, ty, err := edgeTarget(grid, e.Dir, int(x), int(y), blocked)',
    'tx, ty, err := edgeTargetForConnection(grid, e, int(x), int(y), blocked)',
)

replace_once(
    "skill/route_transition.go",
    'tx, ty, err := edgeTarget(water, edge.Dir, int(sx), int(sy), blocked)',
    'tx, ty, err := edgeTargetForConnection(water, edge, int(sx), int(sy), blocked)',
)

Path("world/connection_band.go").write_text(r'''package world

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
''')

Path("world/connection_band_test.go").write_text(r'''package world

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

func TestConnectionEdgesSplitContiguousComponentPairs(t *testing.T) {
	g := &Graph{
		componentAware: true,
		comps: map[uint8][][]int{
			1: {
				{1, 1, 1, 1, 1, 1},
				{1, 1, 1, 1, 1, 1},
			},
			2: {
				{2, 2, 3, 3, 0, 4},
				{2, 2, 3, 3, 0, 4},
			},
		},
		tiles: map[uint8]dim{
			1: {w: 6, h: 2},
			2: {w: 6, h: 2},
		},
		connections: make(map[Edge]rom.Connection),
	}
	c := rom.Connection{Dir: dirNorth, MapID: 2}
	edges := g.connectionEdges(1, c)
	if len(edges) != 3 {
		t.Fatalf("connectionEdges produced %d edges, want 3: %+v", len(edges), edges)
	}

	want := [][2]int{{0, 1}, {2, 3}, {5, 5}}
	wantEntry := []int{2, 3, 4}
	for i, e := range edges {
		start, end, ok := ConnectionBand(e)
		if !ok {
			t.Fatalf("edge %d is not scoped: %+v", i, e)
		}
		if start != want[i][0] || end != want[i][1] {
			t.Errorf("edge %d band = %d..%d, want %d..%d", i, start, end, want[i][0], want[i][1])
		}
		if got := g.connectionPortComps(e, false); len(got) != 1 || got[0] != 1 {
			t.Errorf("edge %d exit components = %v, want [1]", i, got)
		}
		if got := g.connectionPortComps(e, true); len(got) != 1 || got[0] != wantEntry[i] {
			t.Errorf("edge %d entry components = %v, want [%d]", i, got, wantEntry[i])
		}
	}
}

func TestBuildGraphSplitsCeruleanRoute4BorderByLandingComponent(t *testing.T) {
	g := loadGraph(t)
	var edges []Edge
	for _, e := range g.Edges[0x03] {
		if e.Kind == EdgeConnection && e.To == 0x0f {
			edges = append(edges, e)
		}
	}
	if len(edges) < 2 {
		t.Fatalf("Cerulean -> Route 4 has %d connection edge(s), want multiple component-scoped bands", len(edges))
	}

	entries := map[int]bool{}
	for _, e := range edges {
		start, end, ok := ConnectionBand(e)
		if !ok {
			t.Fatalf("Cerulean -> Route 4 edge is not band-scoped: %+v", e)
		}
		if end < start {
			t.Fatalf("invalid band %d..%d on %+v", start, end, e)
		}
		got := g.entryComps[e]
		if len(got) != 1 {
			t.Fatalf("band %d..%d entry components = %v, want exactly one", start, end, got)
		}
		entries[got[0]] = true
	}
	if len(entries) < 2 {
		t.Fatalf("Cerulean -> Route 4 bands land in only %d component(s): %v; want multiple", len(entries), entries)
	}
}
''')

Path("skill/connection_edge.go").write_text(r'''package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/world"
)

// edgeTargetForConnection is edgeTarget constrained to the component-scoped
// source-border band selected by the route graph. This is the execution half
// of world.ConnectionBand: without it the router could choose the good Route 4
// landing component and Traverse would still walk to the nearest tile anywhere
// on the aggregate border, recreating the exact dead-end #307 describes.
func edgeTargetForConnection(g *world.Grid, e world.Edge, sx, sy int, blocked map[[2]int]bool) (int, int, error) {
	start, end, scoped := world.ConnectionBand(e)
	if !scoped {
		return edgeTarget(g, e.Dir, sx, sy, blocked)
	}

	var edge [][2]int
	add := func(i int) {
		switch e.Dir {
		case 0:
			if i >= 0 && i < g.Width {
				edge = append(edge, [2]int{i, 0})
			}
		case 1:
			if i >= 0 && i < g.Width {
				edge = append(edge, [2]int{i, g.Height - 1})
			}
		case 2:
			if i >= 0 && i < g.Height {
				edge = append(edge, [2]int{0, i})
			}
		case 3:
			if i >= 0 && i < g.Height {
				edge = append(edge, [2]int{g.Width - 1, i})
			}
		}
	}
	if e.Dir > 3 {
		return 0, 0, fmt.Errorf("skill: Traverse: unknown connection dir %d", e.Dir)
	}
	for i := start; i <= end; i++ {
		add(i)
	}

	var best [2]int
	bestLen := -1
	for _, t := range edge {
		if !g.Walkable(t[0], t[1]) || blocked[[2]int{t[0], t[1]}] {
			continue
		}
		steps, err := world.FindPath(g, sx, sy, t[0], t[1], blocked)
		if err != nil {
			continue
		}
		if bestLen >= 0 && len(steps) >= bestLen {
			continue
		}
		best, bestLen = t, len(steps)
	}
	if bestLen < 0 {
		return 0, 0, fmt.Errorf("skill: Traverse: no reachable walkable tile on the %s edge band %d..%d from (%d,%d)",
			dirName(e.Dir), start, end, sx, sy)
	}
	return best[0], best[1], nil
}
''')
