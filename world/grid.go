package world

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/rom"
)

// Tileset table layout (pokered.sym: Tilesets = 03:47BE), 12 bytes per entry:
//
//	+0  bank of the tileset's block/gfx data
//	+1  block pointer (2 bytes, little-endian)
//	+3  gfx pointer (2 bytes)
//	+5  collision pointer (2 bytes) -> walkable tile ids, 0xff terminated
//	+7  counter tiles (3 bytes)
//	+10 grass tile
//	+11 animation
const (
	tilesetsBank    uint8  = 0x03
	tilesetsAddr    uint16 = 0x47BE
	tilesetEntryLen        = 12

	// TilePairCollisionsLand (pokered.sym: 00:0c7e) is a bank-0 table of
	// 3-byte {tileset, tileA, tileB} entries terminated by 0xff. The game
	// checks it in CheckForTilePairCollisions on EVERY step: a move between
	// two tiles named by an entry for the current tileset is refused even
	// though BOTH tiles are in the tileset's walkable list. It is how Gen 1
	// builds cave mouths and water edges you can see across but not walk
	// across.
	//
	// MEASURED: Mt. Moon 1F (tileset 17, CAVERN) at (10,22) tile 0x20 with
	// (9,22) tile 0x05 next to it. Both walkable, the pathfinder planned
	// "left", and the game refused the step — entry {17, 0x20, 0x05} is the
	// first row of this table. Without it the planner routes through cave
	// walls it believes are corridors, the walk fails on the first step, and
	// the run bounces between re-plans until its engagement budget dies.
	//
	// ponytail: land table only. TilePairCollisionsWater (00:0ca0, 3 entries)
	// applies while surfing; add it when a run can surf, and pick the table
	// by wWalkBikeSurfState the way CheckForTilePairCollisions2 does.
	tilePairCollisionsLandAddr = 0x0c7e
	tilePairEntryLen           = 3
)

// Grid is a map's collision view, indexed [y][x] in game tile coordinates —
// the same coordinates reported by wXCoord/wYCoord. Each game step covers a
// 2x2 pair of background tiles and the ROM deliberately uses DIFFERENT
// subtiles for two jobs:
//
//   - collision uses the bottom-left tile (row 2*sy+1, column 2*sx), the
//     measured rule this package has always used;
//   - GetTileAndCoordsInFrontOfPlayer uses the top-left tile (row 2*sy,
//     column 2*sx), which is the value field actions such as CUT compare.
//
// Both ids are retained so callers never infer one contract from the other.
type Grid struct {
	MapID         uint8
	Width, Height int // in game tile coordinates
	walkable      []bool
	collisionTile []uint8
	fieldTile     []uint8
	// tilePairs holds this map's tileset's forbidden tile transitions, as a
	// set keyed by the ordered pair. The game's check is symmetric (it
	// matches an entry in either direction), so both orders are stored and
	// callers need not normalise.
	tilePairs map[[2]uint8]bool
}

// Passable reports whether a step from (fx,fy) to (tx,ty) is one the game
// would actually perform: the destination must be in bounds and walkable, and
// the transition must not be a tile-pair collision for this map's tileset.
//
// This is the pathfinding predicate; Walkable alone is the weaker "could the
// player ever stand here". Only orthogonally adjacent arguments are
// meaningful, which is all the searches pass.
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

// tilePairsFor reads the land tile-pair collision table and returns the
// forbidden transitions for one tileset, in both directions.
func tilePairsFor(romData []byte, tileset uint8) map[[2]uint8]bool {
	pairs := map[[2]uint8]bool{}
	for off := tilePairCollisionsLandAddr; off+tilePairEntryLen <= len(romData); off += tilePairEntryLen {
		if romData[off] == 0xff {
			break
		}
		if romData[off] != tileset {
			continue
		}
		a, b := romData[off+1], romData[off+2]
		pairs[[2]uint8{a, b}] = true
		pairs[[2]uint8{b, a}] = true
	}
	return pairs
}

// InBounds reports whether (x, y) is inside the grid.
func (g *Grid) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < g.Width && y < g.Height
}

// Walkable reports whether the game-tile coordinate (x, y) is walkable.
// Out-of-bounds coordinates are not walkable.
func (g *Grid) Walkable(x, y int) bool {
	if !g.InBounds(x, y) {
		return false
	}
	return g.walkable[y*g.Width+x]
}

// Tile returns the bottom-left collision tile id for a game-tile coordinate.
func (g *Grid) Tile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) || len(g.collisionTile) != g.Width*g.Height {
		return 0, false
	}
	return g.collisionTile[y*g.Width+x], true
}

// FieldTile returns the top-left background tile id that
// GetTileAndCoordsInFrontOfPlayer exposes through wTileInFrontOfPlayer when
// this game-coordinate cell is directly in front of the player.
func (g *Grid) FieldTile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) || len(g.fieldTile) != g.Width*g.Height {
		return 0, false
	}
	return g.fieldTile[y*g.Width+x], true
}

// Set sets the walkability of the game-tile coordinate (x, y).
// Out-of-bounds coordinates are ignored.
func (g *Grid) Set(x, y int, ok bool) {
	if !g.InBounds(x, y) {
		return
	}
	g.walkable[y*g.Width+x] = ok
}

// bankedOffset converts a banked address (bank:addr) to a ROM file offset.
func bankedOffset(bank uint8, addr uint16) (int, error) {
	if addr >= 0x4000 {
		return int(bank)*0x4000 + int(addr-0x4000), nil
	}
	if bank != 0 {
		return 0, fmt.Errorf("address %04X in bank %d is below 0x4000", addr, bank)
	}
	return int(addr), nil
}

// Build constructs the collision grid for a parsed map from its immutable ROM
// block map. Runtime navigation should use BuildFromBlocks with the current
// block IDs when map scripts may have replaced blocks after load.
func Build(romData []byte, h rom.MapHeader) (*Grid, error) {
	if h.WidthBlocks == 0 || h.HeightBlocks == 0 {
		return BuildFromBlocks(romData, h, nil)
	}
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		return nil, err
	}
	return BuildFromBlocks(romData, h, blocks)
}

// BuildFromBlocks constructs the collision grid for h using the supplied
// row-major block IDs rather than re-reading the immutable map block data from
// ROM. This is the common decoder for both static maps and the live
// wOverworldMap buffer, so script-driven ReplaceTileBlock changes get exactly
// the same collision/field-tile semantics as ordinary ROM geometry.
func BuildFromBlocks(romData []byte, h rom.MapHeader, blocks []byte) (*Grid, error) {
	width := int(h.WidthBlocks) * 2
	height := int(h.HeightBlocks) * 2
	g := &Grid{
		MapID:         h.ID,
		Width:         width,
		Height:        height,
		walkable:      make([]bool, width*height),
		collisionTile: make([]uint8, width*height),
		fieldTile:     make([]uint8, width*height),
		tilePairs:     tilePairsFor(romData, h.Tileset),
	}
	if width == 0 || height == 0 {
		return g, nil
	}

	wantBlocks := int(h.WidthBlocks) * int(h.HeightBlocks)
	if len(blocks) < wantBlocks {
		return nil, fmt.Errorf("map %d: block map has %d bytes, want at least %d", h.ID, len(blocks), wantBlocks)
	}
	blocks = blocks[:wantBlocks]

	// Look up the map's tileset entry.
	tsOff, err := bankedOffset(tilesetsBank, tilesetsAddr)
	if err != nil {
		return nil, fmt.Errorf("map %d: %v", h.ID, err)
	}
	entryOff := tsOff + int(h.Tileset)*tilesetEntryLen
	if entryOff+tilesetEntryLen > len(romData) {
		return nil, fmt.Errorf("map %d: tileset %d entry at offset %d exceeds ROM of %d bytes", h.ID, h.Tileset, entryOff, len(romData))
	}
	tsBank := romData[entryOff]
	blockPtr := uint16(romData[entryOff+1]) | uint16(romData[entryOff+2])<<8
	collPtr := uint16(romData[entryOff+5]) | uint16(romData[entryOff+6])<<8

	// Read the tileset's walkable-tile list. The list lives in bank 0 (the
	// Home section); the game dereferences it with no bank switch.
	collBank := uint8(0)
	if collPtr >= 0x4000 {
		collBank = tsBank
	}
	collOff, err := bankedOffset(collBank, collPtr)
	if err != nil {
		return nil, fmt.Errorf("map %d: %v", h.ID, err)
	}
	walkableTiles := make([]bool, 256)
	for {
		if collOff >= len(romData) {
			return nil, fmt.Errorf("map %d: collision list at offset %d exceeds ROM of %d bytes", h.ID, collOff, len(romData))
		}
		t := romData[collOff]
		collOff++
		if t == 0xff {
			break
		}
		walkableTiles[t] = true
	}

	// Read the block definitions: 16 tile ids per block, in the tileset bank.
	blockOff, err := bankedOffset(tsBank, blockPtr)
	if err != nil {
		return nil, fmt.Errorf("map %d: %v", h.ID, err)
	}

	wb := int(h.WidthBlocks)
	for by := 0; by < int(h.HeightBlocks); by++ {
		for bx := 0; bx < wb; bx++ {
			blockID := blocks[by*wb+bx]
			tilesOff := blockOff + int(blockID)*16
			if tilesOff+16 > len(romData) {
				return nil, fmt.Errorf("map %d: block %d data at offset %d exceeds ROM of %d bytes", h.ID, blockID, tilesOff, len(romData))
			}
			// Each block is 4x4 background tiles; one game-coordinate cell is
			// the corresponding 2x2 background-tile pair. Collision and field
			// actions intentionally read different left-hand subtiles.
			for sy := 0; sy < 2; sy++ {
				for sx := 0; sx < 2; sx++ {
					field := romData[tilesOff+(2*sy)*4+2*sx]
					collision := romData[tilesOff+(2*sy+1)*4+2*sx]
					i := (by*2+sy)*width + (bx*2 + sx)
					g.fieldTile[i] = field
					g.collisionTile[i] = collision
					g.walkable[i] = walkableTiles[collision]
				}
			}
		}
	}

	return g, nil
}
