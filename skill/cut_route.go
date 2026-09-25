package skill

import (
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
