// Package boxxle defines semantic Boxxle board state independent of raw RAM
// layout.
//
// Boxxle is a Sokoban-style puzzle game. The board is the background tile map
// the game draws (see the sym package): 2x2-tile cells whose kind is named by
// the top-left tile id, with the player as a sprite. Decoding reads VRAM and
// OAM, classifies each cell, and recovers the playable interior by flood fill
// because floor and the exterior share one tile id. The decoder is pure over a
// MemoryReader, so it is testable against synthetic fixtures.
package boxxle

import (
	"errors"
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

	// Tiles holds the top-left tile id of each board cell, relative to the
	// board bounding box, as the game drew it. It is evidence for diagnosis;
	// the Walls/Goals/Crates/Player fields are the classification.
	Tiles [][]byte `json:"tiles,omitempty"`

	// Board cells, relative to the board bounding box. Walls includes cells
	// outside the walled area, which the game draws as plain floor, so a
	// crate can never be planned into them.
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

// ErrMisaligned reports wall, crate or goal cells that do not share one 2x2
// grid alignment, which means the background map is not a board this decoder
// understands.
var ErrMisaligned = errors.New("boxxle: board cells are not on one 2x2 grid")

// minWalls is the fewest wall cells that make a background map a board. It
// keeps title, menu and cutscene screens, which reuse the map, from decoding
// as puzzles.
const minWalls = 8

// cell is one classified 2x2 block of the background map, in tile coordinates.
type cell struct {
	col, row int
	kind     byte // top-left tile id
}

// DecodeState translates the supported Boxxle memory layout into semantic
// state. A screen that is not a board decodes as ScreenUnknown, not an error.
func DecodeState(reader game.MemoryReader) (State, error) {
	if reader == nil {
		return State{}, fmt.Errorf("boxxle: nil memory reader")
	}

	bg := readBackground(reader)
	cells := findCells(bg)
	walls := 0
	for _, c := range cells {
		if c.kind == sym.TileWall {
			walls++
		}
	}
	if walls < minWalls {
		return State{Screen: ScreenUnknown}, nil
	}

	state := State{Screen: ScreenPuzzle}
	if err := decodeBoard(&state, reader, cells); err != nil {
		return State{Screen: ScreenUnknown}, err
	}
	return state, nil
}

// readBackground reads the background map the PPU is currently drawing.
func readBackground(reader game.MemoryReader) [sym.BGMapSize][sym.BGMapSize]byte {
	base := sym.BGMapLow
	if reader.Peek8(sym.LCDC)&sym.LCDCBGMapHigh != 0 {
		base = sym.BGMapHigh
	}
	var bg [sym.BGMapSize][sym.BGMapSize]byte
	for y := 0; y < sym.BGMapSize; y++ {
		reader.PeekInto(base+uint16(y)*sym.BGMapStride, bg[y][:])
	}
	return bg
}

// findCells returns every 2x2 block that is exactly a known cell kind: four
// consecutive tile ids, top-left first.
func findCells(bg [sym.BGMapSize][sym.BGMapSize]byte) []cell {
	var cells []cell
	for row := 0; row+1 < sym.BGMapSize; row++ {
		for col := 0; col+1 < sym.BGMapSize; col++ {
			tl := bg[row][col]
			switch tl {
			case sym.TileWall, sym.TileCrate, sym.TileGoal, sym.TileCrateOnGoal:
			default:
				continue
			}
			if bg[row][col+1] == tl+1 && bg[row+1][col] == tl+2 && bg[row+1][col+1] == tl+3 {
				cells = append(cells, cell{col: col, row: row, kind: tl})
			}
		}
	}
	return cells
}

// decodeBoard classifies cells into walls, goals, crates and the player, then
// seals everything the player cannot reach.
func decodeBoard(state *State, reader game.MemoryReader, cells []cell) error {
	col0, row0 := cells[0].col, cells[0].row
	minX, minY, maxX, maxY := 1<<30, 1<<30, -1, -1
	for _, c := range cells {
		if (c.col-col0)%sym.CellTiles != 0 || (c.row-row0)%sym.CellTiles != 0 {
			return fmt.Errorf("%w: cell at tile (%d,%d) vs origin (%d,%d)", ErrMisaligned, c.col, c.row, col0, row0)
		}
		x, y := floorDiv(c.col-col0, sym.CellTiles), floorDiv(c.row-row0, sym.CellTiles)
		minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
	}

	oam := readOAM(reader)
	scx, scy := int(reader.Peek8(sym.SCX)), int(reader.Peek8(sym.SCY))
	player, hasPlayer := spriteCell(oam[0], scx, scy, col0, row0)
	if hasPlayer {
		minX, minY, maxX, maxY = min(minX, player.X), min(minY, player.Y), max(maxX, player.X), max(maxY, player.Y)
	}
	movedCrates := spriteCrates(oam, scx, scy, col0, row0)
	for _, c := range movedCrates {
		p := c.pos
		minX, minY, maxX, maxY = min(minX, p.X), min(minY, p.Y), max(maxX, p.X), max(maxY, p.Y)
	}

	state.Width, state.Height = maxX-minX+1, maxY-minY+1
	state.Tiles = make([][]byte, state.Height)
	for y := range state.Tiles {
		state.Tiles[y] = make([]byte, state.Width)
		for x := range state.Tiles[y] {
			state.Tiles[y][x] = sym.TileEmpty
		}
	}

	wall := make(map[Pos]bool)
	for _, c := range cells {
		p := Pos{X: floorDiv(c.col-col0, sym.CellTiles) - minX, Y: floorDiv(c.row-row0, sym.CellTiles) - minY}
		state.Tiles[p.Y][p.X] = c.kind
		switch c.kind {
		case sym.TileWall:
			wall[p] = true
		case sym.TileCrate:
			state.Crates = append(state.Crates, p)
		case sym.TileGoal:
			state.Goals = append(state.Goals, p)
		case sym.TileCrateOnGoal:
			state.Crates = append(state.Crates, p)
			state.Goals = append(state.Goals, p)
		}
	}
	for _, c := range movedCrates {
		p := Pos{X: c.pos.X - minX, Y: c.pos.Y - minY}
		state.Crates = append(state.Crates, p)
		if c.onGoal && !contains(state.Goals, p) {
			state.Goals = append(state.Goals, p)
		}
	}
	if hasPlayer {
		p := Pos{X: player.X - minX, Y: player.Y - minY}
		state.Player = &p
	}

	// Floor and exterior draw identically: whatever the player cannot reach
	// (crates and goals are passable for this purpose) is outside the board.
	if state.Player != nil {
		blocked := func(p Pos) bool {
			return p.X < 0 || p.Y < 0 || p.X >= state.Width || p.Y >= state.Height || wall[p]
		}
		reach := map[Pos]bool{*state.Player: true}
		queue := []Pos{*state.Player}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, d := range dirDeltas {
				next := Pos{X: cur.X + d.X, Y: cur.Y + d.Y}
				if !blocked(next) && !reach[next] {
					reach[next] = true
					queue = append(queue, next)
				}
			}
		}
		for y := 0; y < state.Height; y++ {
			for x := 0; x < state.Width; x++ {
				if p := (Pos{X: x, Y: y}); !reach[p] {
					wall[p] = true
				}
			}
		}
	}
	for y := 0; y < state.Height; y++ {
		for x := 0; x < state.Width; x++ {
			if p := (Pos{X: x, Y: y}); wall[p] {
				state.Walls = append(state.Walls, p)
			}
		}
	}

	state.Solved = isSolved(state)
	if state.Solved {
		state.Screen = ScreenSolved
	}
	return nil
}

// sprite is one OAM entry.
type sprite struct{ y, x, tile byte }

func readOAM(reader game.MemoryReader) [sym.OAMEntries]sprite {
	var raw [sym.OAMEntries * sym.OAMEntry]byte
	reader.PeekInto(sym.OAMBase, raw[:])
	var oam [sym.OAMEntries]sprite
	for i := range oam {
		oam[i] = sprite{y: raw[i*sym.OAMEntry], x: raw[i*sym.OAMEntry+1], tile: raw[i*sym.OAMEntry+2]}
	}
	return oam
}

// spriteCell maps the top-left sprite of a 16x16 object onto the board grid.
// It reports false while the sprite is off screen or between cells, so a
// mid-step frame is "not there yet" rather than a wrong cell.
func spriteCell(s sprite, scx, scy, col0, row0 int) (Pos, bool) {
	y, x := int(s.y), int(s.x)
	if y < 16 || y >= 160 || x < 8 || x >= 168 {
		return Pos{}, false
	}
	px := (x - 8 + scx) % (sym.BGMapSize * 8)
	py := (y - 16 + scy) % (sym.BGMapSize * 8)
	if px%8 != 0 || py%8 != 0 {
		return Pos{}, false
	}
	dx, dy := px/8-col0, py/8-row0
	if dx%sym.CellTiles != 0 || dy%sym.CellTiles != 0 {
		return Pos{}, false
	}
	return Pos{X: floorDiv(dx, sym.CellTiles), Y: floorDiv(dy, sym.CellTiles)}, true
}

// spriteCrate is a crate drawn as a sprite.
type spriteCrate struct {
	pos    Pos
	onGoal bool
}

// spriteCrates returns the crates drawn as sprites: four entries carrying the
// crate's consecutive tile ids in a 2x2 block. Entry 0 is the player and is
// skipped.
func spriteCrates(oam [sym.OAMEntries]sprite, scx, scy, col0, row0 int) []spriteCrate {
	var out []spriteCrate
	for i := 1; i < len(oam); i++ {
		tl := oam[i]
		if tl.tile != sym.TileCrate && tl.tile != sym.TileCrateOnGoal {
			continue
		}
		var have [3]bool
		for _, o := range oam[1:] {
			switch {
			case o.tile == tl.tile+1 && o.y == tl.y && o.x == tl.x+8:
				have[0] = true
			case o.tile == tl.tile+2 && o.y == tl.y+8 && o.x == tl.x:
				have[1] = true
			case o.tile == tl.tile+3 && o.y == tl.y+8 && o.x == tl.x+8:
				have[2] = true
			}
		}
		if !have[0] || !have[1] || !have[2] {
			continue
		}
		if p, ok := spriteCell(tl, scx, scy, col0, row0); ok {
			out = append(out, spriteCrate{pos: p, onGoal: tl.tile == sym.TileCrateOnGoal})
		}
	}
	return out
}

func contains(ps []Pos, p Pos) bool {
	for _, q := range ps {
		if q == p {
			return true
		}
	}
	return false
}

// floorDiv is integer division rounding toward negative infinity.
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
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
