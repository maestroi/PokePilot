package world

import (
	"fmt"

	"github.com/maestroi/pokepilot/worldmodel"
)

// NativeStep is one controller-direction displacement on a wide-id grid.
// A ledge hop returns a displacement of two tiles in the chosen direction.
type NativeStep struct {
	DX int
	DY int
}

// NativeGrid is the tile-level counterpart to NativeGraph. It intentionally
// mirrors the mature Grid API without narrowing the cartridge's map identity.
type NativeGrid struct {
	MapID         uint16
	Width, Height int
	walkable      []bool
	collisionTile []uint8
	cuttable      []bool
	blocked       map[uint8]worldmodel.NativeDirectionMask
	jumps         map[uint8]worldmodel.NativeDirectionMask
	Traversal     TraversalMode
}

// NativeGridFromSpec validates and owns an adapter-decoded native grid.
func NativeGridFromSpec(spec worldmodel.NativeGridSpec) (*NativeGrid, error) {
	if spec.Width < 0 || spec.Height < 0 {
		return nil, fmt.Errorf("native map %#04x: negative grid dimensions %dx%d", spec.MapID, spec.Width, spec.Height)
	}
	want := spec.Width * spec.Height
	if len(spec.Walkable) != want || len(spec.CollisionTile) != want {
		return nil, fmt.Errorf("native map %#04x: invalid grid payload for %dx%d", spec.MapID, spec.Width, spec.Height)
	}
	if len(spec.Cuttable) != 0 && len(spec.Cuttable) != want {
		return nil, fmt.Errorf("native map %#04x: invalid cuttable payload for %dx%d", spec.MapID, spec.Width, spec.Height)
	}
	blocked := make(map[uint8]worldmodel.NativeDirectionMask, len(spec.Blocked))
	for collision, mask := range spec.Blocked {
		blocked[collision] = mask
	}
	jumps := make(map[uint8]worldmodel.NativeDirectionMask, len(spec.Jumps))
	for _, jump := range spec.Jumps {
		jumps[jump.Collision] = jump.DirectionMask
	}
	return &NativeGrid{
		MapID: spec.MapID, Width: spec.Width, Height: spec.Height,
		walkable:      append([]bool(nil), spec.Walkable...),
		collisionTile: append([]uint8(nil), spec.CollisionTile...),
		cuttable:      append([]bool(nil), spec.Cuttable...),
		blocked:       blocked, jumps: jumps, Traversal: spec.Traversal,
	}, nil
}

func (g *NativeGrid) InBounds(x, y int) bool {
	return g != nil && x >= 0 && y >= 0 && x < g.Width && y < g.Height
}

func (g *NativeGrid) Walkable(x, y int) bool {
	return g.InBounds(x, y) && g.walkable[y*g.Width+x]
}

func (g *NativeGrid) Tile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) {
		return 0, false
	}
	return g.collisionTile[y*g.Width+x], true
}

func (g *NativeGrid) Cuttable(x, y int) bool {
	return g.InBounds(x, y) && len(g.cuttable) == g.Width*g.Height && g.cuttable[y*g.Width+x]
}

func nativeDirection(step NativeStep) (worldmodel.NativeDirectionMask, worldmodel.NativeDirectionMask, bool) {
	switch {
	case step.DX == 0 && step.DY == 1:
		return worldmodel.NativeBlockDown, worldmodel.NativeBlockUp, true
	case step.DX == 0 && step.DY == -1:
		return worldmodel.NativeBlockUp, worldmodel.NativeBlockDown, true
	case step.DX == -1 && step.DY == 0:
		return worldmodel.NativeBlockLeft, worldmodel.NativeBlockRight, true
	case step.DX == 1 && step.DY == 0:
		return worldmodel.NativeBlockRight, worldmodel.NativeBlockLeft, true
	default:
		return 0, 0, false
	}
}

// Passable applies ordinary one-tile movement, including directional walls on
// both sides of the boundary between source and destination.
func (g *NativeGrid) Passable(fx, fy, tx, ty int) bool {
	step := NativeStep{DX: tx - fx, DY: ty - fy}
	dir, opposite, ok := nativeDirection(step)
	if !ok || !g.Walkable(tx, ty) {
		return false
	}
	from, okFrom := g.Tile(fx, fy)
	to, okTo := g.Tile(tx, ty)
	if !okFrom || !okTo {
		return false
	}
	if g.blocked[from]&dir != 0 || g.blocked[to]&opposite != 0 {
		return false
	}
	return true
}

// Movement models one directional input. Ordinary movement wins first, exactly
// as Gen II's player controller tries TryStep before TryJump. If the adjacent
// tile cannot be entered, a ledge collision on the standing tile may produce a
// two-tile hop in that direction.
func (g *NativeGrid) Movement(x, y int, input NativeStep, occupied map[[2]int]bool) (NativeStep, bool) {
	dir, _, ok := nativeDirection(input)
	if !ok {
		return NativeStep{}, false
	}
	nx, ny := x+input.DX, y+input.DY
	if !occupied[[2]int{nx, ny}] && g.Passable(x, y, nx, ny) {
		return input, true
	}

	from, ok := g.Tile(x, y)
	if !ok || g.jumps[from]&dir == 0 {
		return NativeStep{}, false
	}
	tx, ty := x+2*input.DX, y+2*input.DY
	if occupied[[2]int{tx, ty}] || !g.Walkable(tx, ty) {
		return NativeStep{}, false
	}
	return NativeStep{DX: 2 * input.DX, DY: 2 * input.DY}, true
}

var nativeDirections = []NativeStep{{DX: 1}, {DX: -1}, {DY: 1}, {DY: -1}}

// FindNativePath returns controller inputs from one tile to another. Returned
// steps may contain a two-tile displacement when the route jumps a ledge.
func FindNativePath(g *NativeGrid, sx, sy, tx, ty int, occupied map[[2]int]bool) ([]NativeStep, error) {
	if g == nil {
		return nil, fmt.Errorf("world: nil native grid")
	}
	if !g.InBounds(sx, sy) || !g.InBounds(tx, ty) || !g.Walkable(sx, sy) || !g.Walkable(tx, ty) {
		return nil, ErrNoPath
	}
	if sx == tx && sy == ty {
		return []NativeStep{}, nil
	}
	type node struct {
		x, y int
		prev int
		step NativeStep
	}
	nodes := []node{{x: sx, y: sy, prev: -1}}
	seen := map[[2]int]bool{{sx, sy}: true}

	for i := 0; i < len(nodes); i++ {
		for _, input := range nativeDirections {
			move, ok := g.Movement(nodes[i].x, nodes[i].y, input, occupied)
			if !ok {
				continue
			}
			nx, ny := nodes[i].x+move.DX, nodes[i].y+move.DY
			key := [2]int{nx, ny}
			if seen[key] {
				continue
			}
			seen[key] = true
			nodes = append(nodes, node{x: nx, y: ny, prev: i, step: move})
			j := len(nodes) - 1
			if nx != tx || ny != ty {
				continue
			}
			var path []NativeStep
			for nodes[j].prev >= 0 {
				path = append([]NativeStep{nodes[j].step}, path...)
				j = nodes[j].prev
			}
			return path, nil
		}
	}
	return nil, ErrNoPath
}

// NativeCutApproach is the walkable prefix leading to one Cut obstacle whose
// removal makes the target reachable. Approach stops on the tile beside the
// tree; Cut is the one-tile direction the player must face/use Cut toward.
// Runtime routing replans from fresh live blocks after the cartridge mutates
// the map, so this planner never assumes the replacement collision itself.
type NativeCutApproach struct {
	Approach []NativeStep
	Cut      NativeStep
	TreeX    int
	TreeY    int
}

// FindNativeCutApproach finds one cuttable tile that bridges the current
// walkable component to the target. It evaluates each live cuttable tile by
// making only that tile virtually walkable, asks the ordinary pathfinder
// whether the destination would then connect, and returns the shortest
// pre-Cut approach. At most one tree is assumed removed; a second tree is
// handled by the caller's post-action replan.
func FindNativeCutApproach(g *NativeGrid, sx, sy, tx, ty int, occupied map[[2]int]bool) (NativeCutApproach, error) {
	if g == nil || !g.InBounds(sx, sy) || !g.InBounds(tx, ty) ||
		!g.Walkable(sx, sy) || !g.Walkable(tx, ty) {
		return NativeCutApproach{}, ErrNoPath
	}

	bestCost := int(^uint(0) >> 1)
	var best NativeCutApproach
	found := false
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			if !g.Cuttable(x, y) || occupied[[2]int{x, y}] {
				continue
			}

			virtual := *g
			virtual.walkable = append([]bool(nil), g.walkable...)
			virtual.walkable[y*g.Width+x] = true
			full, err := FindNativePath(&virtual, sx, sy, tx, ty, occupied)
			if err != nil {
				continue
			}

			cx, cy := sx, sy
			for i, step := range full {
				nx, ny := cx+step.DX, cy+step.DY
				if nx == x && ny == y {
					if absNative(step.DX)+absNative(step.DY) != 1 {
						break
					}
					if len(full) < bestCost {
						bestCost = len(full)
						best = NativeCutApproach{
							Approach: append([]NativeStep(nil), full[:i]...),
							Cut:      step,
							TreeX:    x,
							TreeY:    y,
						}
						found = true
					}
					break
				}
				cx, cy = nx, ny
			}
		}
	}
	if !found {
		return NativeCutApproach{}, ErrNoPath
	}
	return best, nil
}

func absNative(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
