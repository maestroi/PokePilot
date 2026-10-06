// Package sym contains the memory layout for Boxxle (USA, Europe) (Rev 1).
//
// Boxxle is a 32 KiB DMG-only cartridge with no external RAM and no MBC. The
// puzzle is not stored in a WRAM array the decoder can rely on; it is the
// background tile map the game draws, so the decoder reads VRAM (measured on
// the real ROM, not assumed):
//
//   - the level is laid out in the 32x32-tile background map, in 2x2-tile
//     (16x16 px) cells;
//   - each cell is four consecutive tile ids, top-left first, so a cell's
//     kind is named by its top-left tile id (Tile* below);
//   - floor and the area outside the walls share one tile id, so the playable
//     interior is recovered by flood fill, not read;
//   - the player is a 16x16 sprite, OAM entry 0, not a background tile;
//   - a crate the player has pushed is also a sprite (the same four tile ids
//     as the background crate) and stays one, so crates are read from both.
package sym

const (
	// BGMapLow and BGMapHigh are the two 32x32 background maps. LCDC bit 3
	// selects which one the PPU draws.
	BGMapLow  uint16 = 0x9800
	BGMapHigh uint16 = 0x9C00
	BGMapSize        = 32
	// BGMapStride is the byte distance between background map rows.
	BGMapStride uint16 = 0x20

	LCDC uint16 = 0xFF40
	SCY  uint16 = 0xFF42
	SCX  uint16 = 0xFF43

	// LCDCBGMapHigh is the LCDC bit that selects BGMapHigh.
	LCDCBGMapHigh byte = 0x08

	// OAMBase is the sprite attribute table: 40 entries of Y, X, tile, flags.
	OAMBase    uint16 = 0xFE00
	OAMEntries        = 40
	OAMEntry          = 4

	// PlayerSprite is OAM entry 0: the top-left 8x8 of the 16x16 player.
	PlayerSprite uint16 = OAMBase
)

// CellTiles is the side of one board cell, in 8x8 tiles.
const CellTiles = 2

// Top-left tile ids of each 2x2 cell kind. The other three tiles of a cell are
// TL+1 (top-right), TL+2 (bottom-left) and TL+3 (bottom-right).
const (
	TileWall  byte = 0xA8
	TileCrate byte = 0xA4
	TileGoal  byte = 0xA0

	// TileCrateOnGoal is a crate standing on a goal.
	TileCrateOnGoal byte = 0xAC

	// TileEmpty is both floor and the exterior; see the package comment.
	TileEmpty byte = 0xD4
)

// Boards too large for 16x16 cells (from the sixth puzzle on) are drawn at
// 8x8 scale instead: one tile per cell, a one-tile player sprite, and these
// cell tile ids, in the same goal/crate/wall/crate-on-goal order as above.
// MEASURED on the real ROM's sixth puzzle: a pushed 0xB9 moves, and pushing
// the crate off 0xBB leaves 0xB8 behind.
const (
	SmallTileGoal        byte = 0xB8
	SmallTileCrate       byte = 0xB9
	SmallTileWall        byte = 0xBA
	SmallTileCrateOnGoal byte = 0xBB
)

// Visible screen size in tiles; the rest of the 32x32 map can hold stale tiles.
const (
	ScreenTilesW int = 20
	ScreenTilesH int = 18
)
