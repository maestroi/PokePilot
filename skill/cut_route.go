package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	overworldTileset uint8 = 0
	gymTileset       uint8 = 7
)

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
