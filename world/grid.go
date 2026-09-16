package world

import "fmt"

// TraversalMode selects a movement-mode-specific collision view. The concrete
// game adapter decides how ROM/runtime data changes between modes; world only
// consumes the resulting forbidden transitions.
type TraversalMode uint8

const (
	TraversalLand TraversalMode = iota
	TraversalWater
)

// Ledge is a directed two-tile hop described in semantic grid terms. The game
// adapter owns the native encoding and translates it into this shape.
type Ledge struct {
	DX, DY     int
	From, Over byte
}

// GridSpec is the game-agnostic input used to construct a collision grid.
// Adapters decode their native map/tileset formats into this immutable shape.
type GridSpec struct {
	MapID         uint8
	Width, Height int
	Walkable      []bool
	CollisionTile []uint8
	FieldTile     []uint8
	TilePairs     map[[2]uint8]bool
	Ledges        []Ledge
	CounterTiles  [3]uint8
}

// Grid is a map's collision view, indexed [y][x] in game tile coordinates.
// Native ROM formats never escape into this type: adapters provide the
// walkability, collision/field tile identities, pair restrictions and ledges.
type Grid struct {
	MapID         uint8
	Width, Height int
	walkable      []bool
	collisionTile []uint8
	fieldTile     []uint8
	tilePairs     map[[2]uint8]bool
	ledges        []Ledge
	counterTiles  [3]uint8
}

// NewGrid validates and copies adapter-decoded grid data so callers cannot
// mutate routing state through retained slices/maps.
func NewGrid(spec GridSpec) (*Grid, error) {
	if spec.Width < 0 || spec.Height < 0 {
		return nil, fmt.Errorf("world: grid %02x has negative dimensions %dx%d", spec.MapID, spec.Width, spec.Height)
	}
	n := spec.Width * spec.Height
	if len(spec.Walkable) != n {
		return nil, fmt.Errorf("world: grid %02x walkability has %d cells, want %d", spec.MapID, len(spec.Walkable), n)
	}
	if len(spec.CollisionTile) != 0 && len(spec.CollisionTile) != n {
		return nil, fmt.Errorf("world: grid %02x collision tiles have %d cells, want %d", spec.MapID, len(spec.CollisionTile), n)
	}
	if len(spec.FieldTile) != 0 && len(spec.FieldTile) != n {
		return nil, fmt.Errorf("world: grid %02x field tiles have %d cells, want %d", spec.MapID, len(spec.FieldTile), n)
	}
	g := &Grid{
		MapID:         spec.MapID,
		Width:         spec.Width,
		Height:        spec.Height,
		walkable:      append([]bool(nil), spec.Walkable...),
		collisionTile: append([]uint8(nil), spec.CollisionTile...),
		fieldTile:     append([]uint8(nil), spec.FieldTile...),
		ledges:        append([]Ledge(nil), spec.Ledges...),
		counterTiles:  spec.CounterTiles,
	}
	if len(spec.TilePairs) > 0 {
		g.tilePairs = make(map[[2]uint8]bool, len(spec.TilePairs))
		for pair, blocked := range spec.TilePairs {
			g.tilePairs[pair] = blocked
		}
	}
	return g, nil
}

// Passable reports whether a step from (fx,fy) to (tx,ty) is one the game
// would actually perform: the destination must be in bounds and walkable, and
// the transition must not be present in the adapter-supplied forbidden-pair set.
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

// Tile returns the adapter-defined collision tile id for a coordinate.
func (g *Grid) Tile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) || len(g.collisionTile) != g.Width*g.Height {
		return 0, false
	}
	return g.collisionTile[y*g.Width+x], true
}

// FieldTile returns the adapter-defined field-action tile id for a coordinate.
func (g *Grid) FieldTile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) || len(g.fieldTile) != g.Width*g.Height {
		return 0, false
	}
	return g.fieldTile[y*g.Width+x], true
}

// IsCounterTile reports whether the collision tile at (x,y) is one of the
// adapter-supplied counter tiles used for extended interaction range.
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

// Set sets the walkability of a game-tile coordinate. It is used by routing
// snapshots to overlay observed blockers without changing adapter facts.
func (g *Grid) Set(x, y int, ok bool) {
	if !g.InBounds(x, y) {
		return
	}
	g.walkable[y*g.Width+x] = ok
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
