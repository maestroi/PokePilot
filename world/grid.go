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
	tilesetEntryLen = 12

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
	tilePairCollisionsLandAddr  = 0x0c7e
	tilePairCollisionsWaterAddr = 0x0ca0
	tilePairEntryLen            = 3
)

// TraversalMode selects the ROM tile-pair table used for movement. Tile
// walkability itself is shared; Gen 1 changes pair restrictions while surfing.
type TraversalMode uint8

const (
	TraversalLand TraversalMode = iota
	TraversalWater
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
	ledges    []rom.Ledge
	// counterTiles are this map's tileset's 3 counter-tile ids (tileset
	// entry +7..+9), 0xff where unused. See IsCounterTile.
	counterTiles [3]uint8
	// tables selects the ROM map/tileset table addresses. Gen I games share
	// one header and tileset format and differ only in these addresses; Red
	// is the default so existing callers are unchanged, and Yellow supplies
	// its own via BuildForTables. Zero value behaves as RedTables.
	tables rom.Tables
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
	return tilePairsForTraversal(romData, tileset, TraversalLand)
}

func tilePairsForTraversal(romData []byte, tileset uint8, mode TraversalMode) map[[2]uint8]bool {
	pairs := map[[2]uint8]bool{}
	addr := tilePairCollisionsLandAddr
	if mode == TraversalWater {
		addr = tilePairCollisionsWaterAddr
	}
	for off := addr; off+tilePairEntryLen <= len(romData); off += tilePairEntryLen {
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

// IsCounterTile reports whether the collision tile at (x, y) is one of this
// map's tileset's counter tiles — the ones IsSpriteOrSignInFrontOfPlayer's
// extendRangeOverCounter (pokered.sym home/overworld.asm:1118,
// wTilesetTalkingOverTiles) lets a player talk across even though the tile
// itself is not walkable. A Pokemon Center or Mart counter is built from
// these: the clerk stands two tiles from the player, with the counter tile
// between, never one tile away as an ordinary NPC does.
//
// MEASURED on Pewter Pokemon Center (map 0x3a): the nurse's counter tile ids
// (0x18,0x19,0x1e) sit in the bottom-left/collision subtile at (3,2), not
// the top-left/field subtile (which reads 0x08, unrelated) — despite the
// field subtile being what CUT and other field actions compare. The
// counter's blocking behavior and its talk-range id are the same subtile.
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
	return BuildForTables(rom.RedTables(), romData, h)
}

// BuildForTables is Build with an explicit set of ROM table addresses. Gen I
// games share one map/tileset format and differ only in these addresses, so
// Yellow passes its own while Red keeps the default.
func BuildForTables(tables rom.Tables, romData []byte, h rom.MapHeader) (*Grid, error) {
	if tables.IsZero() {
		tables = rom.RedTables()
	}
	if h.WidthBlocks == 0 || h.HeightBlocks == 0 {
		return BuildFromBlocksForTables(tables, romData, h, nil, TraversalLand)
	}
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		return nil, err
	}
	return BuildFromBlocksForTables(tables, romData, h, blocks, TraversalLand)
}

// BuildFromBlocks constructs the collision grid for h using the supplied
// row-major block IDs rather than re-reading the immutable map block data from
// ROM. This is the common decoder for both static maps and the live
// wOverworldMap buffer, so script-driven ReplaceTileBlock changes get exactly
// the same collision/field-tile semantics as ordinary ROM geometry.
func BuildFromBlocks(romData []byte, h rom.MapHeader, blocks []byte) (*Grid, error) {
	return BuildFromBlocksForTables(rom.RedTables(), romData, h, blocks, TraversalLand)
}

// BuildFromBlocksForTraversal decodes h with the movement-mode-specific
// tile-pair collision table. It is used by live navigation after Surf changes
// wWalkBikeSurfState; static graph construction intentionally stays on land.
func BuildFromBlocksForTraversal(romData []byte, h rom.MapHeader, blocks []byte, mode TraversalMode) (*Grid, error) {
	return BuildFromBlocksForTables(rom.RedTables(), romData, h, blocks, mode)
}

// BuildFromBlocksForTables is BuildFromBlocksForTraversal with an explicit set
// of ROM table addresses, so a Gen I game other than Red decodes with its own
// tileset table.
func BuildFromBlocksForTables(tables rom.Tables, romData []byte, h rom.MapHeader, blocks []byte, mode TraversalMode) (*Grid, error) {
	if tables.IsZero() {
		tables = rom.RedTables()
	}
	width := int(h.WidthBlocks) * 2
	height := int(h.HeightBlocks) * 2
	g := &Grid{
		MapID:         h.ID,
		Width:         width,
		Height:        height,
		walkable:      make([]bool, width*height),
		collisionTile: make([]uint8, width*height),
		fieldTile:     make([]uint8, width*height),
		tilePairs:     tilePairsForTraversal(romData, h.Tileset, mode),
		ledges:        rom.Ledges(romData, h.Tileset),
		tables:        tables,
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
	tsOff, err := bankedOffset(g.tables.TilesetsBank, g.tables.TilesetsAddr)
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
	g.counterTiles = [3]uint8{romData[entryOff+7], romData[entryOff+8], romData[entryOff+9]}

	// Read the tileset's walkable-tile list. The game dereferences this
	// pointer with no bank switch (_IsTilePassable loads it straight into
	// hl), so the list must already be in the mapped bank. In Red the lists
	// are assembled into bank 0 and every pointer is below 0x4000, which the
	// CPU maps to ROM bank 0 unconditionally. Yellow moved them into bank 1
	// (Overworld_Coll 01:4AC2 vs Red's 00:1735) so its pointers are >= 0x4000
	// and the bank is a build-layout fact the pointer cannot reveal: it is
	// NOT the tileset bank. Tables carries it; a pointer below 0x4000 stays
	// bank 0 in both games.
	collBank := g.tables.CollisionListBank
	if collPtr < 0x4000 {
		collBank = 0
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
