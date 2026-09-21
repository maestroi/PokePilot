package controller

import (
	"container/heap"
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

type yellowFieldPathAction uint8

const (
	yellowFieldWalk yellowFieldPathAction = iota
	yellowFieldCut
	yellowFieldSurf
)

type yellowFieldPathStep struct {
	Move   world.Step
	Action yellowFieldPathAction
}

type yellowFieldPathState struct {
	x, y  int
	water bool
}

type yellowFieldPathCost struct {
	actions int
	moves   int
}

func (c yellowFieldPathCost) less(other yellowFieldPathCost) bool {
	if c.actions != other.actions {
		return c.actions < other.actions
	}
	return c.moves < other.moves
}

type yellowFieldPathParent struct {
	prev yellowFieldPathState
	step yellowFieldPathStep
}

type yellowFieldPathNode struct {
	state yellowFieldPathState
	cost  yellowFieldPathCost
	index int
}

type yellowFieldPathQueue []*yellowFieldPathNode

func (q yellowFieldPathQueue) Len() int { return len(q) }
func (q yellowFieldPathQueue) Less(i, j int) bool {
	return q[i].cost.less(q[j].cost)
}
func (q yellowFieldPathQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}
func (q *yellowFieldPathQueue) Push(v any) {
	n := v.(*yellowFieldPathNode)
	n.index = len(*q)
	*q = append(*q, n)
}
func (q *yellowFieldPathQueue) Pop() any {
	old := *q
	n := len(old)
	v := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return v
}

type yellowLiveSprite struct {
	slot      int
	x, y      int
	pictureID uint8
}

const (
	yellowSpriteSlotSize    uint16 = 0x10
	yellowSpritePictureID   uint16 = 0x00
	yellowSpriteImageIndex  uint16 = 0x02
	yellowSpriteMapY        uint16 = 0x04
	yellowSpriteMapX        uint16 = 0x05
	yellowBoulderPictureID         = 0x49
)

func yellowLiveSprites(m *emu.Emu) []yellowLiveSprite {
	var out []yellowLiveSprite
	for slot := 1; slot <= 15; slot++ {
		data1 := sym.SpritePlayerStateData1 + uint16(slot)*yellowSpriteSlotSize
		data2 := sym.SpriteStateData2 + uint16(slot)*yellowSpriteSlotSize
		picture := m.Peek8(data1 + yellowSpritePictureID)
		if picture == 0 || m.Peek8(data1+yellowSpriteImageIndex) == 0xff {
			continue
		}
		out = append(out, yellowLiveSprite{
			slot:      slot,
			x:         int(m.Peek8(data2+yellowSpriteMapX)) - 4,
			y:         int(m.Peek8(data2+yellowSpriteMapY)) - 4,
			pictureID: picture,
		})
	}
	return out
}

func yellowLiveObjectBlockers(m *emu.Emu, extra map[[2]int]bool) map[[2]int]bool {
	out := make(map[[2]int]bool, len(extra)+15)
	for at, blocked := range extra {
		if blocked {
			out[at] = true
		}
	}
	for _, sprite := range yellowLiveSprites(m) {
		out[[2]int{sprite.x, sprite.y}] = true
	}
	return out
}

func yellowCutTile(g *world.Grid, tileset uint8, x, y int) bool {
	match := func(tile uint8) bool {
		switch tileset {
		case 0: // OVERWORLD
			return tile == 0x3d || tile == 0x52
		case 7: // GYM
			return tile == 0x50
		default:
			return false
		}
	}
	if tile, ok := g.FieldTile(x, y); ok && match(tile) {
		return true
	}
	if tile, ok := g.Tile(x, y); ok && match(tile) {
		return true
	}
	return false
}

func yellowWaterTile(g *world.Grid, x, y int) bool {
	if tile, ok := g.FieldTile(x, y); ok && tile == yellowSurfWaterTile {
		return true
	}
	if tile, ok := g.Tile(x, y); ok && tile == yellowSurfWaterTile {
		return true
	}
	return false
}

func planYellowFieldPath(
	land, water *world.Grid,
	tileset uint8,
	sx, sy, dx, dy int,
	blocked map[[2]int]bool,
	canCut, canSurf, startWater bool,
) ([]yellowFieldPathStep, error) {
	if land == nil || !land.InBounds(sx, sy) || !land.InBounds(dx, dy) ||
		blocked[[2]int{sx, sy}] || blocked[[2]int{dx, dy}] {
		return nil, world.ErrNoPath
	}
	if sx == dx && sy == dy {
		return []yellowFieldPathStep{}, nil
	}

	start := yellowFieldPathState{x: sx, y: sy, water: startWater}
	best := map[yellowFieldPathState]yellowFieldPathCost{start: {}}
	parent := map[yellowFieldPathState]yellowFieldPathParent{}
	closed := map[yellowFieldPathState]bool{}
	open := &yellowFieldPathQueue{}
	heap.Init(open)
	heap.Push(open, &yellowFieldPathNode{state: start})

	push := func(from, to yellowFieldPathState, step yellowFieldPathStep, moveCost int) {
		if closed[to] {
			return
		}
		next := best[from]
		next.moves += moveCost
		if step.Action != yellowFieldWalk {
			next.actions++
		}
		if old, ok := best[to]; ok && !next.less(old) {
			return
		}
		best[to] = next
		parent[to] = yellowFieldPathParent{prev: from, step: step}
		heap.Push(open, &yellowFieldPathNode{state: to, cost: next})
	}

	dirs := []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight}
	for open.Len() > 0 {
		node := heap.Pop(open).(*yellowFieldPathNode)
		cur := node.state
		if closed[cur] || node.cost != best[cur] {
			continue
		}
		closed[cur] = true
		if cur.x == dx && cur.y == dy {
			var rev []yellowFieldPathStep
			for at := cur; at != start; {
				p, ok := parent[at]
				if !ok {
					return nil, world.ErrNoPath
				}
				rev = append(rev, p.step)
				at = p.prev
			}
			out := make([]yellowFieldPathStep, len(rev))
			for i := range rev {
				out[len(rev)-1-i] = rev[i]
			}
			return out, nil
		}

		for _, input := range dirs {
			if cur.water {
				move, ok := water.Movement(cur.x, cur.y, input, blocked)
				if !ok {
					continue
				}
				nx, ny := cur.x+move.DX, cur.y+move.DY
				nextWater := yellowWaterTile(water, nx, ny) || !land.Walkable(nx, ny)
				push(cur, yellowFieldPathState{x: nx, y: ny, water: nextWater},
					yellowFieldPathStep{Move: move, Action: yellowFieldWalk},
					yellowControllerAbsInt(move.DX)+yellowControllerAbsInt(move.DY))
				continue
			}

			if move, ok := land.Movement(cur.x, cur.y, input, blocked); ok {
				nx, ny := cur.x+move.DX, cur.y+move.DY
				push(cur, yellowFieldPathState{x: nx, y: ny},
					yellowFieldPathStep{Move: move, Action: yellowFieldWalk},
					yellowControllerAbsInt(move.DX)+yellowControllerAbsInt(move.DY))
				continue
			}

			nx, ny := cur.x+input.DX, cur.y+input.DY
			if !land.InBounds(nx, ny) || blocked[[2]int{nx, ny}] {
				continue
			}
			if canCut && yellowCutTile(land, tileset, nx, ny) {
				push(cur, yellowFieldPathState{x: nx, y: ny},
					yellowFieldPathStep{Move: input, Action: yellowFieldCut}, 1)
				continue
			}
			if canSurf && yellowWaterTile(water, nx, ny) {
				if move, ok := water.Movement(cur.x, cur.y, input, blocked); ok {
					wx, wy := cur.x+move.DX, cur.y+move.DY
					push(cur, yellowFieldPathState{x: wx, y: wy, water: true},
						yellowFieldPathStep{Move: move, Action: yellowFieldSurf},
						yellowControllerAbsInt(move.DX)+yellowControllerAbsInt(move.DY))
				}
			}
		}
	}
	return nil, world.ErrNoPath
}

func yellowCurrentFieldPath(m *emu.Emu, romData []byte, h yellowrom.MapHeader, tx, ty int, extra map[[2]int]bool) ([]yellowFieldPathStep, error) {
	land, err := yellowLiveMapGridForTraversal(m, romData, h, world.TraversalLand)
	if err != nil {
		return nil, err
	}
	water, err := yellowLiveMapGridForTraversal(m, romData, h, world.TraversalWater)
	if err != nil {
		return nil, err
	}
	blocked := yellowLiveObjectBlockers(m, extra)
	delete(blocked, [2]int{int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord))})

	cut := fieldCapabilityFor(m, romData, FieldCut)
	surf := fieldCapabilityFor(m, romData, FieldSurf)
	return planYellowFieldPath(
		land, water, h.Tileset,
		int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord)), tx, ty,
		blocked,
		cut.Usable || cut.Preparable,
		surf.Usable || surf.Preparable,
		m.Peek8(sym.WalkBikeSurfState) == 2,
	)
}

func yellowFirstFieldAction(plan []yellowFieldPathStep) ([]world.Step, *yellowFieldPathStep) {
	prefix := make([]world.Step, 0, len(plan))
	for i := range plan {
		if plan[i].Action != yellowFieldWalk {
			step := plan[i]
			return prefix, &step
		}
		prefix = append(prefix, plan[i].Move)
	}
	return prefix, nil
}

func faceYellowStep(m *emu.Emu, step world.Step) error {
	if yellowControllerAbsInt(step.DX)+yellowControllerAbsInt(step.DY) != 1 {
		return fmt.Errorf("yellow travel: cannot face non-adjacent step %+v", step)
	}
	btn, ok := buttonFor(step)
	if !ok {
		return fmt.Errorf("yellow travel: invalid facing step %+v", step)
	}
	m.Tap(btn, 3, 7)
	return nil
}

func executeYellowFieldPathAction(m *emu.Emu, romData []byte, mapID uint8, step yellowFieldPathStep) error {
	if err := faceYellowStep(m, step.Move); err != nil {
		return err
	}
	switch step.Action {
	case yellowFieldCut:
		if err := UseFieldMove(m, romData, FieldCut); err != nil {
			return fmt.Errorf("yellow travel: Cut: %w", err)
		}
		if err := walkPath(m, mapID, []world.Step{step.Move}); err != nil {
			return fmt.Errorf("yellow travel: enter cleared Cut tile: %w", err)
		}
		return nil
	case yellowFieldSurf:
		if err := UseFieldMove(m, romData, FieldSurf); err != nil {
			return fmt.Errorf("yellow travel: Surf: %w", err)
		}
		if m.Peek8(sym.WalkBikeSurfState) != 2 {
			return fmt.Errorf("yellow travel: Surf returned without surfing state")
		}
		return nil
	default:
		return fmt.Errorf("yellow travel: attempted to execute ordinary field-path step")
	}
}

func yellowWalkToWithFieldActions(m *emu.Emu, romData []byte, tx, ty int, extra map[[2]int]bool) error {
	mapID := m.Peek8(sym.CurMap)
	for replan := 0; replan < 16; replan++ {
		if int(m.Peek8(sym.XCoord)) == tx && int(m.Peek8(sym.YCoord)) == ty {
			return nil
		}
		h, err := yellowrom.ParseMap(romData, mapID)
		if err != nil {
			return err
		}
		plan, err := yellowCurrentFieldPath(m, romData, h, tx, ty, extra)
		if err != nil {
			return err
		}
		prefix, action := yellowFirstFieldAction(plan)
		if len(prefix) > 0 {
			if err := walkPath(m, mapID, prefix); err != nil {
				return err
			}
		}
		if action == nil {
			return nil
		}
		if err := executeYellowFieldPathAction(m, romData, mapID, *action); err != nil {
			return err
		}
		if m.Peek8(sym.CurMap) != mapID {
			return fmt.Errorf("yellow travel: field action unexpectedly changed map %#02x -> %#02x", mapID, m.Peek8(sym.CurMap))
		}
	}
	return fmt.Errorf("yellow travel: exceeded local field-action replan budget toward (%d,%d)", tx, ty)
}

func yellowMaybeFlash(m *emu.Emu, romData []byte) error {
	if m.Peek8(sym.MapPalOffset) == 0 {
		return nil
	}
	cap := fieldCapabilityFor(m, romData, FieldFlash)
	if !cap.Usable && !cap.Preparable {
		return nil
	}
	if err := UseFieldMove(m, romData, FieldFlash); err != nil {
		return fmt.Errorf("yellow travel: auto-Flash: %w", err)
	}
	if m.Peek8(sym.MapPalOffset) != 0 {
		return errors.New("yellow travel: Flash returned without clearing dark-map palette offset")
	}
	return nil
}
