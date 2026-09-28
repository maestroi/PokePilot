// Package sym contains RAM symbols for Boxxle (USA, Europe) (Rev 1).
//
// Boxxle is a 32 KiB DMG-only cartridge with no external RAM and no MBC, so all
// game state lives in WRAM. The board is the video-mode-1 background tile map:
// each screen cell is one tile id, and the tile graphics in VRAM map a tile id
// to a rendered cell. Decoding the board means reading the tile map and
// classifying each tile id.
package sym

const (
	// TileMapTopLeft is the top-left cell of the video-mode-1 background tile
	// map. Rows are separated by TileMapStride bytes; the visible board is
	// TileMapWidth columns by TileMapHeight rows.
	TileMapTopLeft uint16 = 0xC000
	TileMapStride  uint16 = 0x20
	TileMapWidth          = 20
	TileMapHeight         = 18

	// GameStateBase is the start of the WRAM block the game updates as it
	// advances through title, menu, and puzzle screens. The exact field
	// offsets within this block are revision-specific and are decoded by
	// boxxle/state.go.
	GameStateBase uint16 = 0xC300
)

const (
	// EmptyTile is the background tile id: a screen cell with this id is
	// empty (no wall, goal, crate, or player).
	EmptyTile byte = 0x00
)
