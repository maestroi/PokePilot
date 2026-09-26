package world

import (
	"fmt"

	"github.com/maestroi/pokepilot/worldmodel"
)

// TraversalMode selects movement-specific collision semantics supplied by the
// active game adapter.
type TraversalMode = worldmodel.TraversalMode
type MapID = worldmodel.MapID

const (
	TraversalLand  = worldmodel.TraversalLand
	TraversalWater = worldmodel.TraversalWater
)

// Grid is a map's collision view, indexed [y][x] in game tile coordinates.
// Collision decoding is adapter-owned; generic pathfinding only consumes the
// portable result below.
type Grid struct {
	MapID         MapID
	Width, Height int
	walkable      []bool
	collisionTile []uint8
	fieldTile     []uint8
	cuttable      []bool
	tilePairs     map[[2]uint8]bool
	ledges        []worldmodel.Ledge
	counterTiles  [3]uint8
	// Traversal is the movement mode the collision view was decoded for.
	Traversal TraversalMode
}

// Passable reports whether a step from (fx,fy) to (tx,ty) is one the game
// would actually perform according to the decoded collision model.
func (g *Grid) Passable(fx, fy, tx, ty int) bool {
	if !g.Walkable(tx, ty) {
		return false
	}
	if len(g.tilePairs) == 0 {
		return true
	}
	from, okFrom := g.Tile(fx, fy)
	to, okTo := g.Tile(tx, ty)
	if !okFrom || !okTo {
		return true
	}
	return !g.tilePairs[[2]uint8{from, to}]
}

// InBounds reports whether (x, y) is inside the grid.
func (g *Grid) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < g.Width && y < g.Height
}

// Walkable reports whether the game-tile coordinate (x, y) is walkable.
func (g *Grid) Walkable(x, y int) bool {
	if !g.InBounds(x, y) {
		return false
	}
	return g.walkable[y*g.Width+x]
}

// Tile returns the adapter's collision tile id for a game-tile coordinate.
func (g *Grid) Tile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) || len(g.collisionTile) != g.Width*g.Height {
		return 0, false
	}
	return g.collisionTile[y*g.Width+x], true
}

// FieldTile returns the adapter's field-action tile id for a game coordinate.
func (g *Grid) FieldTile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) || len(g.fieldTile) != g.Width*g.Height {
		return 0, false
	}
	return g.fieldTile[y*g.Width+x], true
}

// Cuttable reports whether the active adapter marks (x,y) as a removable
// Cut-style route obstacle. Games that do not expose such cells return false.
func (g *Grid) Cuttable(x, y int) bool {
	if !g.InBounds(x, y) || len(g.cuttable) != g.Width*g.Height {
		return false
	}
	return g.cuttable[y*g.Width+x]
}

// IsCounterTile reports whether the collision tile at (x, y) is one of the
// adapter-declared counter tiles.
func (g *Grid) IsCounterTile(x, y int) bool {
	t, ok := g.Tile(x, y)
	if !ok {
		return false
	}
	for _, c := range g.counterTiles {
		if c == t {
			return true
		}
	}
	return false
}

// Set sets the walkability of the game-tile coordinate (x, y).
func (g *Grid) Set(x, y int, ok bool) {
	if !g.InBounds(x, y) {
		return
	}
	g.walkable[y*g.Width+x] = ok
}

// Build constructs collision geometry through the map header's adapter-owned
// decoder. Existing callers can keep passing their concrete map-header type as
// long as it implements worldmodel.GridHeader.
func Build(romData []byte, h worldmodel.GridHeader) (*Grid, error) {
	return BuildFromBlocksForTraversal(romData, h, nil, TraversalLand)
}

// BuildFromBlocks uses live row-major block ids while retaining the adapter's
// collision semantics.
func BuildFromBlocks(romData []byte, h worldmodel.GridHeader, blocks []byte) (*Grid, error) {
	return BuildFromBlocksForTraversal(romData, h, blocks, TraversalLand)
}

// BuildFromBlocksForTraversal decodes geometry for the requested movement
// mode. The game adapter owns ROM layout and returns only a portable GridSpec.
func BuildFromBlocksForTraversal(romData []byte, h worldmodel.GridHeader, blocks []byte, mode TraversalMode) (*Grid, error) {
	spec, err := h.WorldGridSpec(romData, blocks, mode)
	if err != nil {
		return nil, err
	}
	return gridFromSpec(spec)
}

// GridFromSpec builds a Grid from a portable adapter GridSpec.
func GridFromSpec(spec worldmodel.GridSpec) (*Grid, error) {
	return gridFromSpec(spec)
}

func gridFromSpec(spec worldmodel.GridSpec) (*Grid, error) {
	if spec.Width < 0 || spec.Height < 0 {
		return nil, fmt.Errorf("map %d: negative grid dimensions %dx%d", spec.MapID, spec.Width, spec.Height)
	}
	want := spec.Width * spec.Height
	if len(spec.Walkable) != want || len(spec.CollisionTile) != want || len(spec.FieldTile) != want {
		return nil, fmt.Errorf("map %d: invalid grid payload for %dx%d", spec.MapID, spec.Width, spec.Height)
	}
	pairs := make(map[[2]uint8]bool, len(spec.TilePairs))
	for pair, blocked := range spec.TilePairs {
		pairs[pair] = blocked
	}
	return &Grid{
		MapID:         spec.MapID,
		Width:         spec.Width,
		Height:        spec.Height,
		walkable:      append([]bool(nil), spec.Walkable...),
		collisionTile: append([]uint8(nil), spec.CollisionTile...),
		fieldTile:     append([]uint8(nil), spec.FieldTile...),
		cuttable:      append([]bool(nil), spec.Cuttable...),
		tilePairs:     pairs,
		ledges:        append([]worldmodel.Ledge(nil), spec.Ledges...),
		counterTiles:  spec.CounterTiles,
		Traversal:     spec.Traversal,
	}, nil
}

// Movement returns the displacement for an input, including directed hops.
// Obstacles are current observations; neither a landing nor the intervening
// tile may contain an object. Ordinary movement retains Passable's contract.
func (g *Grid) Movement(x, y int, input Step, blocked map[[2]int]bool) (Step, bool) {
	nx, ny := x+input.DX, y+input.DY
	if blocked[[2]int{nx, ny}] {
		return Step{}, false
	}
	from, _ := g.Tile(x, y)
	over, _ := g.Tile(nx, ny)
	for _, l := range g.ledges {
		if input.DX == l.DX && input.DY == l.DY && from == l.From && over == l.Over {
			tx, ty := nx+input.DX, ny+input.DY
			return Step{2 * input.DX, 2 * input.DY}, g.Walkable(tx, ty) && !blocked[[2]int{tx, ty}]
		}
	}
	return input, g.Passable(x, y, nx, ny)
}
