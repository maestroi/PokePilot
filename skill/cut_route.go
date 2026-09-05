package skill

import (
	"errors"
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	overworldTileset uint8 = 0
	gymTileset       uint8 = 7
)

type routeCutCandidate struct {
	x, y int
	d    int
}

// cutRouteTile reports whether the TOP-LEFT field-action tile is a tree the
// game's CUT field move can remove in this tileset. The tile id alone is not
// enough: UsedCut first checks the tileset (OVERWORLD or GYM), then compares
// wTileInFrontOfPlayer with $3d or $50 respectively.
func cutRouteTile(tileset, tile uint8) bool {
	switch tileset {
	case overworldTileset:
		return tile == cutTreeTile
	case gymTileset:
		return tile == gymCutTreeTile
	default:
		return false
	}
}

// cutCapabilityRecoverable is the route-facing field-capability query. A
// learned Cut + Cascade Badge is immediately usable. An owned HM is only
// enough when the generic TM/HM policy can actually teach it to the current
// party; HM01 in the bag by itself is deliberately not a capability.
func cutCapabilityRecoverable(romData []byte, mem *state.Mem) bool {
	cap := FieldCapabilityFor(mem, FieldCut)
	return cap.Usable || CanPrepareFieldMove(romData, mem, FieldCut)
}

func routeCutCandidates(grid *world.Grid, tileset uint8, sx, sy int) []routeCutCandidate {
	out := make([]routeCutCandidate, 0)
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			tile, ok := grid.FieldTile(x, y)
			if !ok || !cutRouteTile(tileset, tile) {
				continue
			}
			out = append(out, routeCutCandidate{
				x: x,
				y: y,
				d: absInt(x-sx) + absInt(y-sy),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].d != out[j].d {
			return out[i].d < out[j].d
		}
		if out[i].y != out[j].y {
			return out[i].y < out[j].y
		}
		return out[i].x < out[j].x
	})
	return out
}

func reachableBesideOnMap(grid *world.Grid, mapID uint8, sx, sy, tx, ty int, blocked map[[2]int]bool) (Destination, bool) {
	bestLen := int(^uint(0) >> 1)
	var best Destination
	found := false
	for _, s := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		x, y := tx+s.DX, ty+s.DY
		if !grid.InBounds(x, y) || !grid.Walkable(x, y) {
			continue
		}
		steps, err := world.FindPath(grid, sx, sy, x, y, blocked)
		if err == nil && len(steps) < bestLen {
			bestLen = len(steps)
			found = true
			best = Destination{Map: mapID, X: uint8(x), Y: uint8(y)}
		}
	}
	return best, found
}

// cutThroughReachableTree removes one real, reachable Cut tree on the current
// map and steps onto the cleared cell. Stepping onto it is load-bearing: the
// route planner rebuilds collision from immutable ROM on its next attempt,
// but FindPath deliberately permits a solid START cell so a player standing
// on a live-mutated tree cell can leave it for the newly opened side.
//
// The static grid is used only to identify candidates whose TOP-LEFT field
// tile is the exact tree id for this tileset — the same subtile the ROM puts
// in wTileInFrontOfPlayer. Walkability still comes from the independently
// measured bottom-left collision tile. Before any field move is used the live
// game must agree through wTileInFrontOfPlayer, so an ordinary wall is never
// guessed to be removable.
func cutThroughReachableTree(m *emu.Emu, romData []byte) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !cutCapabilityRecoverable(romData, &mem) {
		return false, nil
	}
	cur := mem.U8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return false, fmt.Errorf("skill: cut route: parse map %02x: %w", cur, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return false, fmt.Errorf("skill: cut route: build map %02x: %w", cur, err)
	}

	sx, sy := playerXY(m)
	for _, c := range routeCutCandidates(grid, h.Tileset, int(sx), int(sy)) {
		sx, sy = playerXY(m)
		stand, ok := reachableBesideOnMap(grid, cur, int(sx), int(sy), c.x, c.y, spriteBlockers(m))
		if !ok {
			continue
		}
		if err := GoTo(m, romData, stand); err != nil {
			// Battles and dialogue belong to Travel's existing recovery loop.
			// Bubble them out unchanged so the caller can resolve them and try
			// the same route again from settled RAM.
			if errors.Is(err, ErrBattle) || errors.Is(err, ErrDialogueInterrupted) {
				return false, err
			}
			continue
		}
		if err := Face(m, uint8(c.x), uint8(c.y)); err != nil {
			continue
		}
		m.StepFrames(2)
		if !cuttableFrontTile(m.Peek8(sym.TileInFrontOfPlayer)) {
			continue
		}

		px, py := playerXY(m)
		step := world.Step{DX: c.x - int(px), DY: c.y - int(py)}
		if absInt(step.DX)+absInt(step.DY) != 1 {
			return false, fmt.Errorf("skill: cut route: tree (%d,%d) is not adjacent to player (%d,%d)", c.x, c.y, px, py)
		}
		if _, err := UseFieldMove(m, FieldCut); err != nil {
			return false, fmt.Errorf("skill: cut route: cut tree at (%d,%d): %w", c.x, c.y, err)
		}
		if err := StepOnce(m, step); err != nil {
			return false, fmt.Errorf("skill: cut route: tree at (%d,%d) was cut but cleared cell could not be entered: %w", c.x, c.y, err)
		}
		return true, nil
	}
	return false, nil
}
