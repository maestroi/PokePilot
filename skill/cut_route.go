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

// cutRouteTile reports whether a background subtile is a tree the game's CUT
// field move can remove in this tileset. UsedCut first checks the tileset
// (OVERWORLD or GYM), then compares wTileInFrontOfPlayer with $3d or $50.
// Which subtile that RAM byte reads depends on facing; callers must check
// both FieldTile and the collision tile, then confirm live after Face.
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

func cellCutRouteTile(grid *world.Grid, tileset uint8, x, y int) bool {
	if field, ok := grid.FieldTile(x, y); ok && cutRouteTile(tileset, field) {
		return true
	}
	// GetTileAndCoordsInFrontOfPlayer reads a screen subtile that is not
	// always the top-left FieldTile. Vermilion's gym tree stores $3d on the
	// collision (bottom-left) subtile; FieldTile-only matching never sees it.
	if coll, ok := grid.Tile(x, y); ok && cutRouteTile(tileset, coll) {
		return true
	}
	return false
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
			if grid.Walkable(x, y) || !cellCutRouteTile(grid, tileset, x, y) {
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

func buttonForFacing(f state.Facing) (emu.Button, bool) {
	switch f {
	case state.FacingUp:
		return emu.Up, true
	case state.FacingDown:
		return emu.Down, true
	case state.FacingLeft:
		return emu.Left, true
	case state.FacingRight:
		return emu.Right, true
	}
	return 0, false
}

// observeFrontTile asks the ROM to refresh wTileInFrontOfPlayer for the
// direction the player is already facing. Face only writes the sprite
// direction; GetTileAndCoordsInFrontOfPlayer runs when the overworld
// considers a step.
func observeFrontTile(m *emu.Emu) uint8 {
	var mem state.Mem
	state.Snapshot(m, &mem)
	btn, ok := buttonForFacing(state.DecodePlayer(&mem).Facing)
	if !ok {
		return m.Peek8(sym.TileInFrontOfPlayer)
	}
	m.Tap(btn, 3, 7)
	m.StepFrames(8)
	return m.Peek8(sym.TileInFrontOfPlayer)
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
// The static grid identifies candidates whose field or collision subtile is
// the tileset's Cut-tree id. GetTileAndCoordsInFrontOfPlayer does not always
// read the top-left FieldTile, so collision must be considered too. Before
// any field move is used the live game must agree through a refreshed
// wTileInFrontOfPlayer, so an ordinary wall is never guessed to be removable.
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
		if !cuttableFrontTile(observeFrontTile(m)) {
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
