package world

import (
	"fmt"

	"github.com/maestroi/pokepilot/worldmodel"
)

type EdgeKind uint8

const (
	EdgeWarp EdgeKind = iota
	EdgeConnection
)

type Edge struct {
	Kind  EdgeKind
	From  uint8
	To    uint8
	WarpX uint8
	WarpY uint8
	Dir   uint8

	BandStart  uint8
	BandEnd    uint8
	BandScoped bool
}

const (
	dirNorth = 0
	dirSouth = 1
	dirWest  = 2
	dirEast  = 3
)

type dim struct{ w, h int }

type Graph struct {
	Edges map[uint8][]Edge

	componentAware bool
	comps          map[uint8][][]int
	exitComps      map[Edge][]int
	entryComps     map[Edge][]int
	warps          map[uint8][]worldmodel.Warp
	tiles          map[uint8]dim
	connections    map[Edge]worldmodel.Connection
	reachable      map[uint8]map[int][]int
	provider       worldmodel.MapHeaderProvider
}

// BuildGraph builds a map-level graph from an adapter-supplied provider. A
// raw []byte ROM is accepted as a compatibility bridge and is resolved through
// registered game adapters; new callers should pass MapHeaderProvider directly.
func BuildGraph(source any) (*Graph, error) {
	provider, err := graphProvider(source)
	if err != nil {
		return nil, err
	}
	return buildGraph(provider)
}

func graphProvider(source any) (worldmodel.MapHeaderProvider, error) {
	switch v := source.(type) {
	case worldmodel.MapHeaderProvider:
		if v == nil {
			return nil, fmt.Errorf("nil map header provider")
		}
		return v, nil
	case []byte:
		provider, ok := worldmodel.ProviderForROM(v)
		if !ok {
			return nil, fmt.Errorf("no registered map provider for ROM of %d bytes", len(v))
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("unsupported map graph source %T", source)
	}
}

func buildGraph(provider worldmodel.MapHeaderProvider) (*Graph, error) {
	headers := make(map[uint8]worldmodel.MapHeader)
	for _, id := range provider.MapIDs() {
		h, err := provider.ParseMap(id)
		if err != nil {
			continue
		}
		headers[id] = h
	}
	if len(headers) == 0 {
		return nil, fmt.Errorf("map provider returned no parseable maps")
	}

	warpTo := make(map[uint8]map[uint8]bool)
	explicit := make(map[uint8]map[uint8]bool)
	for id, h := range headers {
		for _, w := range h.Warps {
			if w.Inert {
				continue
			}
			if warpTo[w.DestMap] == nil {
				warpTo[w.DestMap] = make(map[uint8]bool)
			}
			warpTo[w.DestMap][id] = true
			if w.DestMap != 0xFF {
				if explicit[id] == nil {
					explicit[id] = make(map[uint8]bool)
				}
				explicit[id][w.DestMap] = true
			}
		}
	}

	g := &Graph{
		Edges:          make(map[uint8][]Edge, len(headers)),
		componentAware: true,
		comps:          make(map[uint8][][]int, len(headers)),
		exitComps:      make(map[Edge][]int),
		entryComps:     make(map[Edge][]int),
		warps:          make(map[uint8][]worldmodel.Warp, len(headers)),
		tiles:          make(map[uint8]dim, len(headers)),
		connections:    make(map[Edge]worldmodel.Connection),
		reachable:      make(map[uint8]map[int][]int),
		provider:       provider,
	}
	for id, h := range headers {
		g.Edges[id] = nil
		for _, c := range h.Connections {
			g.connections[Edge{Kind: EdgeConnection, From: id, To: c.MapID, Dir: c.Dir}] = c
		}
		g.warps[id] = h.Warps
		g.tiles[id] = dim{w: int(h.WidthBlocks) * 2, h: int(h.HeightBlocks) * 2}
		if spec, err := provider.Grid(id, nil, worldmodel.TraversalLand); err == nil {
			if grid, err := gridFromSpec(spec); err == nil {
				// Warp tiles are walkable floor, but stepping on one leaves the
				// map. Flood-filling through them falsely merges rooms that are
				// only joined by a teleporter pad (Silph Co 5F's Card Key
				// corridor across (9,15)). Component analysis must match
				// GoTo's warpAvoidance: pads are ports, not corridors.
				g.comps[id] = componentsWithBlocked(grid, warpTileBlockers(h.Warps))
				g.reachable[id] = componentReachability(grid, g.comps[id])
			}
		}
	}

	resolve := func(a uint8, w worldmodel.Warp) (uint8, bool) {
		if w.DestMap != 0xFF {
			return w.DestMap, true
		}
		n, dest := 0, uint8(0)
		for b := range warpTo[a] {
			if explicit[a][b] {
				continue
			}
			n++
			dest = b
		}
		if n == 1 {
			return dest, true
		}
		if n > 1 {
			if d := nearestDir(int(w.X), int(w.Y), g.tiles[a].w, g.tiles[a].h); d >= 0 {
				var match uint8
				matched := 0
				for b := range warpTo[a] {
					if explicit[a][b] {
						continue
					}
					if g.candidateSide(b, a) == d {
						match = b
						matched++
					}
				}
				if matched == 1 {
					return match, true
				}
			}
		}
		return 0, false
	}

	for id, h := range headers {
		if elevator, ok := provider.LookupElevator(id); ok {
			for _, w := range h.Warps {
				if w.Inert {
					continue
				}
				for _, floor := range elevator.Floors {
					g.Edges[id] = append(g.Edges[id], Edge{
						Kind: EdgeWarp, From: id, To: floor.MapID, WarpX: w.X, WarpY: w.Y,
					})
				}
			}
		} else {
			for _, w := range h.Warps {
				if w.Inert {
					continue
				}
				to, ok := resolve(id, w)
				if !ok {
					continue
				}
				g.Edges[id] = append(g.Edges[id], Edge{
					Kind: EdgeWarp, From: id, To: to, WarpX: w.X, WarpY: w.Y,
				})
			}
		}
		for _, c := range h.Connections {
			g.Edges[id] = append(g.Edges[id], g.connectionEdges(id, c)...)
		}
	}

	for _, es := range g.Edges {
		for _, e := range es {
			g.exitComps[e] = g.exitPortComps(e)
			g.entryComps[e] = g.expandComponents(e.To, g.entryPortComps(e))
		}
	}
	return g, nil
}

// Components is the exported form of components, for callers outside this
// package that need to know which tiles of a map are actually reachable from
// one another rather than just walkable.
func Components(grid *Grid) [][]int {
	return components(grid)
}

func warpTileBlockers(warps []worldmodel.Warp) map[[2]int]bool {
	if len(warps) == 0 {
		return nil
	}
	out := make(map[[2]int]bool, len(warps))
	for _, w := range warps {
		if w.Inert {
			continue
		}
		out[[2]int{int(w.X), int(w.Y)}] = true
	}
	return out
}

func components(grid *Grid) [][]int {
	return componentsWithBlocked(grid, nil)
}

// componentsWithBlocked is components, but tiles in blocked are treated as
// non-walkable for the flood. Callers use this to keep active teleporter/door
// warp pads from bridging rooms that can only be joined by actually taking the
// warp edge. Inert warp-table entries are deliberately not blocked.
func componentsWithBlocked(grid *Grid, blocked map[[2]int]bool) [][]int {
	w, h := grid.Width, grid.Height
	comps := make([][]int, h)
	for y := range comps {
		comps[y] = make([]int, w)
	}
	walkable := func(x, y int) bool {
		if blocked[[2]int{x, y}] {
			return false
		}
		return grid.Walkable(x, y)
	}
	next := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !walkable(x, y) || comps[y][x] != 0 {
				continue
			}
			next++
			comps[y][x] = next
			queue := [][2]int{{x, y}}
			for qi := 0; qi < len(queue); qi++ {
				c := queue[qi]
				for _, dd := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					nx, ny := c[0]+dd[0], c[1]+dd[1]
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					if !walkable(nx, ny) || comps[ny][nx] != 0 {
						continue
					}
					if !grid.Passable(c[0], c[1], nx, ny) {
						continue
					}
					comps[ny][nx] = next
					queue = append(queue, [2]int{nx, ny})
				}
			}
		}
	}
	return comps
}

func (g *Graph) exitPortComps(e Edge) []int {
	comps := g.comps[e.From]
	if comps == nil {
		return nil
	}
	d := g.tiles[e.From]
	switch e.Kind {
	case EdgeConnection:
		if _, ok := g.connections[e]; ok {
			return g.connectionPortComps(e, false)
		}
		return edgeLineComps(comps, d.w, d.h, e.Dir)
	case EdgeWarp:
		return tileOrNeighbourComps(comps, d.w, d.h, int(e.WarpX), int(e.WarpY))
	}
	return nil
}

func (g *Graph) entryPortComps(e Edge) []int {
	comps := g.comps[e.To]
	if comps == nil {
		return nil
	}
	d := g.tiles[e.To]
	switch e.Kind {
	case EdgeConnection:
		if _, ok := g.connections[e]; ok {
			return g.connectionPortComps(e, true)
		}
		return edgeLineComps(comps, d.w, d.h, uint8(oppositeDir(int(e.Dir))))
	case EdgeWarp:
		dx, dy, ok := g.destWarpTile(e)
		if !ok {
			return nil
		}
		return tileOrNeighbourComps(comps, d.w, d.h, dx, dy)
	}
	return nil
}

func (g *Graph) connectionPortComps(e Edge, arrival bool) []int {
	c := g.connections[e]
	src, dst := g.tiles[e.From], g.tiles[e.To]
	n := src.w
	if e.Dir >= dirWest {
		n = src.h
	}
	var out []int
	seen := map[int]bool{}
	start, end := connectionBandRange(e, n)
	for i := start; i <= end; i++ {
		j := i + int(c.Offset)
		sx, sy, tx, ty := i, 0, j, dst.h-1
		switch e.Dir {
		case dirSouth:
			sy, ty = src.h-1, 0
		case dirWest:
			sx, sy, tx, ty = 0, i, dst.w-1, j
		case dirEast:
			sx, sy, tx, ty = src.w-1, i, 0, j
		}
		a, b := standingComponentAt(g, e.From, sx, sy), standingComponentAt(g, e.To, tx, ty)
		if len(a) == 0 || len(b) == 0 {
			continue
		}
		v := a[0]
		if arrival {
			v = b[0]
		}
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func (g *Graph) destWarpTile(e Edge) (int, int, bool) {
	var destID int
	if g.provider != nil {
		if floor, ok := g.provider.ElevatorFloorForDestination(e.From, e.To); ok {
			destID = int(floor.DestWarpID)
			dest := g.warps[e.To]
			if destID >= len(dest) {
				return 0, 0, false
			}
			return int(dest[destID].X), int(dest[destID].Y), true
		}
	}
	found := false
	for _, w := range g.warps[e.From] {
		if int(w.X) == int(e.WarpX) && int(w.Y) == int(e.WarpY) {
			destID = int(w.DestWarpID)
			found = true
			break
		}
	}
	if !found {
		return 0, 0, false
	}
	dest := g.warps[e.To]
	if destID >= len(dest) {
		return 0, 0, false
	}
	return int(dest[destID].X), int(dest[destID].Y), true
}

func edgeLineComps(comps [][]int, w, h int, dir uint8) []int {
	seen := make(map[int]bool)
	var out []int
	add := func(x, y int) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		if c := comps[y][x]; c != 0 && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	for i := 0; i < w; i++ {
		if dir == dirNorth {
			add(i, 0)
		}
		if dir == dirSouth {
			add(i, h-1)
		}
	}
	for j := 0; j < h; j++ {
		if dir == dirWest {
			add(0, j)
		}
		if dir == dirEast {
			add(w-1, j)
		}
	}
	return out
}

func tileOrNeighbourComps(comps [][]int, w, h int, x, y int) []int {
	seen := make(map[int]bool)
	var out []int
	add := func(x, y int) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		if c := comps[y][x]; c != 0 && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	if x >= 0 && y >= 0 && x < w && y < h && comps[y][x] != 0 {
		add(x, y)
		return out
	}
	add(x+1, y)
	add(x-1, y)
	add(x, y+1)
	add(x, y-1)
	return out
}

func nearestDir(x, y, w, h int) int {
	type cand struct{ dir, d int }
	cands := []cand{{dirNorth, y}, {dirSouth, h - 1 - y}, {dirWest, x}, {dirEast, w - 1 - x}}
	best := cands[0].d
	for _, c := range cands {
		if c.d < best {
			best = c.d
		}
	}
	dir, count := -1, 0
	for _, c := range cands {
		if c.d == best {
			dir, count = c.dir, count+1
		}
	}
	if count != 1 {
		return -1
	}
	return dir
}

func oppositeDir(d int) int {
	switch d {
	case dirNorth:
		return dirSouth
	case dirSouth:
		return dirNorth
	case dirWest:
		return dirEast
	case dirEast:
		return dirWest
	}
	return -1
}

func (g *Graph) candidateSide(cand, m uint8) int {
	side := -1
	for _, cw := range g.warps[cand] {
		if cw.DestMap != m {
			continue
		}
		d := g.tiles[cand]
		face := nearestDir(int(cw.X), int(cw.Y), d.w, d.h)
		if face < 0 {
			return -1
		}
		s := oppositeDir(face)
		if s < 0 {
			return -1
		}
		if side == -1 {
			side = s
		} else if side != s {
			return -1
		}
	}
	return side
}
