package world

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/rom"
)

// maxMapID is the highest map id present in the Red ROM header tables.
const maxMapID = 0xF7

type EdgeKind uint8

const (
	EdgeWarp EdgeKind = iota // step into a warp tile
	EdgeConnection           // walk off the map edge
)

type Edge struct {
	Kind   EdgeKind
	From   uint8 // source map id
	To     uint8 // destination map id, already resolved (never 0xFF)
	WarpX  uint8 // EdgeWarp only: the warp tile on the source map
	WarpY  uint8
	Dir    uint8 // EdgeConnection only: 0=north 1=south 2=west 3=east
}

// Map-edge directions, matching rom.Connection.Dir and Edge.Dir.
const (
	dirNorth = 0
	dirSouth = 1
	dirWest  = 2
	dirEast  = 3
)

// dim is a map's size in game tiles (WidthBlocks*2, HeightBlocks*2).
type dim struct{ w, h int }

type Graph struct {
	Edges map[uint8][]Edge

	// Component-aware routing data, populated by BuildGraph. componentAware is
	// false for hand-built graphs (tests), in which case FindRoute* imposes no
	// component or walkability constraint and behaves exactly as before.
	componentAware bool
	comps          map[uint8][][]int // per-map component labels, 0 = not walkable
	exitComps      map[Edge][]int    // components an edge's exit port touches (on e.From)
	entryComps     map[Edge][]int    // components an edge's entry port touches (on e.To)
	warps          map[uint8][]rom.Warp
	tiles          map[uint8]dim
}

// BuildGraph builds a MAP-level graph over every parseable map. Nodes are map
// ids; edges are warps and edge connections. Tile arrival coordinates are not
// stored: they are resolved at execution time from live RAM.
//
// A warp whose DestMap is 0xFF means "the map you came from". It is resolved
// statically to the unique map that has a warp back to the source, excluding
// any map the source already reaches via an explicit warp (that direction is
// covered by its own edge). If zero or more than one candidate remains the
// edge is dropped rather than guessed, so no edge ever has To == 0xFF.
func BuildGraph(romData []byte) (*Graph, error) {
	headers := make(map[uint8]rom.MapHeader)
	for id := uint8(0); id <= maxMapID; id++ {
		h, err := rom.ParseMap(romData, id)
		if err != nil {
			continue // invalid map id: skip, do not fail the build
		}
		headers[id] = h
	}
	if len(headers) == 0 {
		return nil, fmt.Errorf("no parseable maps in ROM of %d bytes", len(romData))
	}

	// warpTo[A] is the set of maps that have a warp (any DestWarpID) to A.
	warpTo := make(map[uint8]map[uint8]bool)
	// explicit[A] is the set of maps A reaches via a non-0xFF warp.
	explicit := make(map[uint8]map[uint8]bool)
	for id, h := range headers {
		for _, w := range h.Warps {
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
		warps:          make(map[uint8][]rom.Warp, len(headers)),
		tiles:          make(map[uint8]dim, len(headers)),
	}
	for id, h := range headers {
		g.Edges[id] = nil // a node exists for every valid map, even edge-less ones
		g.warps[id] = h.Warps
		g.tiles[id] = dim{w: int(h.WidthBlocks) * 2, h: int(h.HeightBlocks) * 2}
		if grid, err := Build(romData, h); err == nil {
			g.comps[id] = components(grid)
		}
		// else: this map's block data is corrupt and Build fails; it has no
		// walkable grid, so its edges are not component-constrained (canExit
		// returns true when the map has no grid). ParseMap still accepted the
		// header, so the node and its edges exist.
	}

	resolve := func(a uint8, w rom.Warp) (uint8, bool) {
		if w.DestMap != 0xFF {
			return w.DestMap, true
		}
		n, dest := 0, uint8(0)
		for b := range warpTo[a] {
			if explicit[a][b] {
				continue // a already warps explicitly to b
			}
			n++
			dest = b
		}
		if n == 1 {
			return dest, true
		}
		if n > 1 {
			// A gate shared by several maps (the Route 22 gate 0xC1 sits between
			// Route 22 and Route 23: every door's DestMap is 0xFF and both routes
			// warp back to it). Disambiguate by geometry: a door leads to the
			// candidate on the side it faces. See candidateSide.
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
		return 0, false // zero or ambiguous: drop, do not guess
	}

	for id, h := range headers {
		for _, w := range h.Warps {
			to, ok := resolve(id, w)
			if !ok {
				continue
			}
			g.Edges[id] = append(g.Edges[id], Edge{
				Kind:  EdgeWarp,
				From:  id,
				To:    to,
				WarpX: w.X,
				WarpY: w.Y,
			})
		}
		for _, c := range h.Connections {
			g.Edges[id] = append(g.Edges[id], Edge{
				Kind: EdgeConnection,
				From: id,
				To:   c.MapID,
				Dir:  c.Dir,
			})
		}
	}

	// Port component sets, for the component-aware leg predicate in FindRoute*.
	for _, es := range g.Edges {
		for _, e := range es {
			g.exitComps[e] = g.exitPortComps(e)
			g.entryComps[e] = g.entryPortComps(e)
		}
	}
	return g, nil
}

// components labels each walkable tile of grid with a 1-based component id
// (0 = not walkable) via 4-directional flood fill. A map with disconnected
// walkable regions (a ledge, a wall, a gate) gets one id per region.
func components(grid *Grid) [][]int {
	w, h := grid.Width, grid.Height
	comps := make([][]int, h)
	for y := range comps {
		comps[y] = make([]int, w)
	}
	next := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !grid.Walkable(x, y) || comps[y][x] != 0 {
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
					if !grid.Walkable(nx, ny) || comps[ny][nx] != 0 {
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

// exitPortComps returns the components that edge e's exit port touches on
// e.From: for a connection, the walkable tiles on the map edge in e.Dir; for a
// warp, the warp tile (or its walkable neighbours, as with a solid stair).
func (g *Graph) exitPortComps(e Edge) []int {
	comps := g.comps[e.From]
	if comps == nil {
		return nil
	}
	d := g.tiles[e.From]
	switch e.Kind {
	case EdgeConnection:
		return edgeLineComps(comps, d.w, d.h, e.Dir)
	case EdgeWarp:
		return tileOrNeighbourComps(comps, d.w, d.h, int(e.WarpX), int(e.WarpY))
	}
	return nil
}

// entryPortComps returns the components that edge e's entry port touches on
// e.To: for a connection, the walkable tiles on the opposite map edge; for a
// warp, the destination warp tile (or its walkable neighbours).
func (g *Graph) entryPortComps(e Edge) []int {
	comps := g.comps[e.To]
	if comps == nil {
		return nil
	}
	d := g.tiles[e.To]
	switch e.Kind {
	case EdgeConnection:
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

// destWarpTile finds the destination warp tile on e.To for a warp edge: the
// warp on e.From at (e.WarpX, e.WarpY) names a DestWarpID, and the warp at that
// index on e.To is where the player lands.
func (g *Graph) destWarpTile(e Edge) (int, int, bool) {
	var destID int
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

// edgeLineComps returns the components of the walkable tiles on a map's edge
// in the given direction.
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

// tileOrNeighbourComps returns the component of (x, y) if walkable, else the
// components of its walkable 4-neighbours (the player pushes a solid stair
// from an adjacent tile).
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
	add(x + 1, y)
	add(x - 1, y)
	add(x, y + 1)
	add(x, y - 1)
	return out
}

// nearestDir returns the direction (dirNorth/...) from (x, y) toward the
// nearest map edge, or -1 if the nearest edge is a tie (ambiguous).
func nearestDir(x, y, w, h int) int {
	type cand struct{ dir, d int }
	cands := []cand{
		{dirNorth, y},
		{dirSouth, h - 1 - y},
		{dirWest, x},
		{dirEast, w - 1 - x},
	}
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

// oppositeDir maps a direction to the one pointing the other way.
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

// candidateSide returns the side of map m that cand is on (a dir* constant),
// computed from cand's warp(s) to m: the door on cand that leads to m faces
// toward m, so cand lies on the opposite side. Returns -1 if ambiguous (no
// door, a tie, or doors facing different ways).
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
