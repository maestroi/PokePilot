// Package boxxle defines semantic Boxxle board state independent of raw RAM
// layout.
//
// Boxxle is a Sokoban-style puzzle game. The board is the video-mode-1
// background tile map at 0xC000: each screen cell is one tile id, and the tile
// graphics in VRAM map a tile id to a rendered cell. Decoding the board means
// reading the tile map and classifying each tile id into a cell type (wall,
// goal, crate, player, or empty).
//
// The tile-id→cell-type mapping is not fixed: the game loads a different tile
// set per screen. The decoder therefore classifies cells structurally — by
// position (border → wall) and by tile-id frequency (singleton → player,
// small clusters → goals, medium clusters → crates) — rather than by a
// hardcoded tile table. This keeps the decoder ROM-free and testable against
// synthetic fixtures.
package boxxle

import (
	"fmt"

	"github.com/maestroi/pokepilot/boxxle/sym"
	"github.com/maestroi/pokepilot/game"
)

// Screen is the lifecycle phase the cartridge is in.
type Screen string

const (
	ScreenUnknown    Screen = "unknown"
	ScreenTitle      Screen = "title"
	ScreenMenu       Screen = "menu"
	ScreenPuzzle     Screen = "puzzle"
	ScreenSolved     Screen = "solved"
	ScreenTransition Screen = "transition"
)

// Pos is a board cell coordinate, origin at the top-left of the board
// bounding box.
type Pos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// State is the semantic snapshot of a Boxxle board. Board is populated only
// when Screen is Puzzle or Solved; on other screens the board fields are zero.
type State struct {
	Screen Screen `json:"screen"`

	// Board dimensions, in cells. Zero when the board is not visible.
	Width  int `json:"width"`
	Height int `json:"height"`

	// Tiles is the raw tile map, relative to the board bounding box. This is
	// the ground truth: Tiles[y][x] is the tile id the game renders at that
	// position. The Walls/Goals/Crates/Player fields are a best-effort
	// structural classification of Tiles; callers that need exact cell types
	// should classify Tiles themselves using the VRAM tile graphics.
	Tiles [][]byte `json:"tiles,omitempty"`

	// Board cells, relative to the board bounding box. These are a
	// best-effort structural classification of Tiles.
	Walls  []Pos `json:"walls,omitempty"`
	Goals  []Pos `json:"goals,omitempty"`
	Crates []Pos `json:"crates,omitempty"`
	Player *Pos  `json:"player,omitempty"`

	// Solved is true when every crate sits on a goal.
	Solved bool `json:"solved"`
}

// StateProfile is the optional gameplay-state capability implemented by the
// Boxxle cartridge profile. It deliberately extends CartridgeProfile rather
// than the Pokémon-specific game.GameProfile.
type StateProfile interface {
	game.CartridgeProfile
	DecodeBoxxleState(game.MemoryReader) (State, error)
}

// Observe decodes Boxxle state through a cartridge capability without teaching
// generic cartridge selection about Boxxle-specific fields.
func Observe(profile game.CartridgeProfile, reader game.MemoryReader) (State, error) {
	decoder, ok := profile.(StateProfile)
	if !ok {
		if profile == nil {
			return State{}, fmt.Errorf("boxxle: nil cartridge profile")
		}
		return State{}, fmt.Errorf("boxxle: cartridge profile %s@%s does not expose Boxxle state", profile.ID(), profile.Revision())
	}
	return decoder.DecodeBoxxleState(reader)
}

// DecodeState translates the supported Boxxle RAM layout into semantic state.
func DecodeState(reader game.MemoryReader) (State, error) {
	if reader == nil {
		return State{}, fmt.Errorf("boxxle: nil memory reader")
	}

	tileMap := readTileMap(reader)
	screen := detectScreen(tileMap)

	state := State{Screen: screen}

	if screen == ScreenPuzzle || screen == ScreenSolved {
		decodeBoard(&state, tileMap)
	}

	return state, nil
}

// readTileMap reads the video-mode-1 background tile map from RAM.
func readTileMap(reader game.MemoryReader) [sym.TileMapHeight][sym.TileMapWidth]byte {
	var tm [sym.TileMapHeight][sym.TileMapWidth]byte
	for y := 0; y < sym.TileMapHeight; y++ {
		row := sym.TileMapTopLeft + uint16(y)*sym.TileMapStride
		for x := 0; x < sym.TileMapWidth; x++ {
			tm[y][x] = reader.Peek8(row + uint16(x))
		}
	}
	return tm
}

// detectScreen classifies the lifecycle phase from the tile map structure.
//
// The classification is structural, not tile-id-based. The key discriminator
// is border density: a puzzle board has a fully-filled border (walls), while
// the title and menu screens do not.
//
//   - Puzzle: the bounding-box border is ≥90% filled and the box is compact
//     (not the full screen).
//   - Menu: the bounding box is tall and narrow (height ≥ 2×width).
//   - Title: the bounding box spans most of the screen.
//   - Unknown: none of the above.
func detectScreen(tm [sym.TileMapHeight][sym.TileMapWidth]byte) Screen {
	b, ok := nonEmptyBounds(tm)
	if !ok {
		return ScreenUnknown
	}

	width := b.maxX - b.minX + 1
	height := b.maxY - b.minY + 1

	// Border density: fraction of bounding-box border cells that are non-empty.
	borderCells := 2*width + 2*(height-2)
	if borderCells < 0 {
		borderCells = 0
	}
	borderFilled := 0
	for x := b.minX; x <= b.maxX; x++ {
		if tm[b.minY][x] != sym.EmptyTile {
			borderFilled++
		}
		if tm[b.maxY][x] != sym.EmptyTile {
			borderFilled++
		}
	}
	for y := b.minY + 1; y < b.maxY; y++ {
		if tm[y][b.minX] != sym.EmptyTile {
			borderFilled++
		}
		if tm[y][b.maxX] != sym.EmptyTile {
			borderFilled++
		}
	}
	borderDensity := 0
	if borderCells > 0 {
		borderDensity = borderFilled * 100 / borderCells
	}

	// A puzzle board is a compact rectangle (at least 3×3) with a nearly-full
	// border.
	if borderDensity >= 90 && width >= 3 && height >= 3 &&
		width < sym.TileMapWidth && height < sym.TileMapHeight {
		return ScreenPuzzle
	}

	// The menu is tall and narrow (a vertical list of text tiles).
	if height >= width*2 {
		return ScreenMenu
	}

	// The title screen spans most of the screen.
	if width >= sym.TileMapWidth-2 || height >= sym.TileMapHeight-2 {
		return ScreenTitle
	}

	return ScreenUnknown
}

// nonEmptyBounds returns the bounding box of non-empty tiles.
type bounds struct {
	minX, minY, maxX, maxY int
}

func nonEmptyBounds(tm [sym.TileMapHeight][sym.TileMapWidth]byte) (bounds, bool) {
	b := bounds{minX: sym.TileMapWidth, minY: sym.TileMapHeight, maxX: -1, maxY: -1}
	found := false
	for y := 0; y < sym.TileMapHeight; y++ {
		for x := 0; x < sym.TileMapWidth; x++ {
			if tm[y][x] != sym.EmptyTile {
				if x < b.minX {
					b.minX = x
				}
				if x > b.maxX {
					b.maxX = x
				}
				if y < b.minY {
					b.minY = y
				}
				if y > b.maxY {
					b.maxY = y
				}
				found = true
			}
		}
	}
	return b, found
}

// decodeBoard classifies the tile map into walls, goals, crates, and player.
//
// Classification is structural:
//   - Border cells of the bounding box are walls.
//   - Interior cells are classified by tile-id frequency: the singleton tile
//     is the player, tiles appearing 2–4 times are goals, tiles appearing
//     5+ times are crates, and the rest are empty.
func decodeBoard(state *State, tm [sym.TileMapHeight][sym.TileMapWidth]byte) {
	b, ok := nonEmptyBounds(tm)
	if !ok {
		return
	}

	width := b.maxX - b.minX + 1
	height := b.maxY - b.minY + 1
	state.Width = width
	state.Height = height

	// Populate the raw tile map (ground truth).
	state.Tiles = make([][]byte, height)
	for y := 0; y < height; y++ {
		state.Tiles[y] = make([]byte, width)
		for x := 0; x < width; x++ {
			state.Tiles[y][x] = tm[b.minY+y][b.minX+x]
		}
	}

	// Count tile-id frequency in the interior (excluding the border).
	freq := make(map[byte]int)
	for y := b.minY + 1; y < b.maxY; y++ {
		for x := b.minX + 1; x < b.maxX; x++ {
			tile := tm[y][x]
			if tile != sym.EmptyTile {
				freq[tile]++
			}
		}
	}

	// Identify the player tile: the interior tile id that appears exactly
	// once. If multiple tile ids appear once, pick the first (deterministic).
	var playerTile byte
	playerFound := false
	for tile := byte(1); ; tile++ {
		if freq[tile] == 1 {
			playerTile = tile
			playerFound = true
			break
		}
		if tile == 0xFF {
			break
		}
	}

	// Classify each cell.
	for y := b.minY; y <= b.maxY; y++ {
		for x := b.minX; x <= b.maxX; x++ {
			tile := tm[y][x]
			if tile == sym.EmptyTile {
				continue
			}
			pos := Pos{X: x - b.minX, Y: y - b.minY}

			// Border cells are walls.
			if x == b.minX || x == b.maxX || y == b.minY || y == b.maxY {
				state.Walls = append(state.Walls, pos)
				continue
			}

			// Interior cells.
			if playerFound && tile == playerTile {
				p := pos
				state.Player = &p
				continue
			}
			switch {
			case freq[tile] >= 2 && freq[tile] <= 4:
				state.Goals = append(state.Goals, pos)
			case freq[tile] >= 5:
				state.Crates = append(state.Crates, pos)
			default:
				// freq[tile] == 1 but not the player: treat as a goal
				// (a single goal is common in Sokoban).
				state.Goals = append(state.Goals, pos)
			}
		}
	}

	// Solved: every crate sits on a goal.
	state.Solved = isSolved(state)
}

// isSolved reports whether every crate occupies a goal cell.
func isSolved(state *State) bool {
	if len(state.Crates) == 0 {
		return false
	}
	goalSet := make(map[Pos]bool, len(state.Goals))
	for _, g := range state.Goals {
		goalSet[g] = true
	}
	for _, c := range state.Crates {
		if !goalSet[c] {
			return false
		}
	}
	return true
}
