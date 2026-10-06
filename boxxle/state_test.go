package boxxle

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/boxxle/sym"
)

// fakeReader is a ROM-free MemoryReader backed by a 64 KiB address space.
type fakeReader struct {
	mem []byte
}

func newFakeReader() *fakeReader { return &fakeReader{mem: make([]byte, 0x10000)} }

func (f *fakeReader) Peek8(addr uint16) byte { return f.mem[addr] }

func (f *fakeReader) PeekInto(addr uint16, dst []byte) {
	for i := range dst {
		dst[i] = f.mem[addr+uint16(i)]
	}
}

// boardOrigin is where fixtures place cell (0,0) in the 32x32 background map,
// in tiles. Real levels are not anchored at the map corner (level 1-1 starts at
// tile column 1), so the fixtures are not either.
const boardOriginCol, boardOriginRow = 1, 2

// newBoardReader lays out a Sokoban text board the way the ROM draws it: 2x2
// background cells, floor and exterior both the empty tile, and the player as
// OAM sprite 0. Legend: # wall, $ crate, . goal, * crate on goal, @ player,
// + player on goal, space floor. Crates drawn as sprites are added separately.
func newBoardReader(rows ...string) *fakeReader {
	f := newFakeReader()
	f.mem[sym.LCDC] = 0x91
	for y := 0; y < sym.BGMapSize; y++ {
		for x := 0; x < sym.BGMapSize; x++ {
			f.mem[int(sym.BGMapLow)+y*sym.BGMapSize+x] = sym.TileEmpty
		}
	}
	for y, row := range rows {
		for x, ch := range row {
			switch ch {
			case '#':
				f.putCell(x, y, sym.TileWall)
			case '$':
				f.putCell(x, y, sym.TileCrate)
			case '.', '+':
				f.putCell(x, y, sym.TileGoal)
			case '*':
				f.putCell(x, y, sym.TileCrateOnGoal)
			}
			if ch == '@' || ch == '+' {
				f.setPlayer(x, y)
			}
		}
	}
	return f
}

func (f *fakeReader) putCell(x, y int, tl byte) {
	col, row := boardOriginCol+2*x, boardOriginRow+2*y
	base := int(sym.BGMapLow)
	f.mem[base+row*sym.BGMapSize+col] = tl
	f.mem[base+row*sym.BGMapSize+col+1] = tl + 1
	f.mem[base+(row+1)*sym.BGMapSize+col] = tl + 2
	f.mem[base+(row+1)*sym.BGMapSize+col+1] = tl + 3
}

// spritePx is the OAM (y, x) of cell (x, y)'s top-left sprite.
func spritePx(x, y int) (byte, byte) {
	return byte(16 + (boardOriginRow+2*y)*8), byte(8 + (boardOriginCol+2*x)*8)
}

func (f *fakeReader) setPlayer(x, y int) {
	oy, ox := spritePx(x, y)
	f.mem[sym.PlayerSprite], f.mem[sym.PlayerSprite+1], f.mem[sym.PlayerSprite+2] = oy, ox, 0x98
}

// putSpriteCrate draws a crate as four sprites in OAM slots 4-7.
func (f *fakeReader) putSpriteCrate(x, y int, tile byte) {
	oy, ox := spritePx(x, y)
	at := int(sym.OAMBase) + 4*sym.OAMEntry
	for i, o := range [][2]byte{{0, 0}, {0, 8}, {8, 0}, {8, 8}} {
		f.mem[at+i*sym.OAMEntry] = oy + o[0]
		f.mem[at+i*sym.OAMEntry+1] = ox + o[1]
		f.mem[at+i*sym.OAMEntry+2] = tile + byte(i)
	}
}

func mustDecode(t *testing.T, f *fakeReader) State {
	t.Helper()
	state, err := DecodeState(f)
	if err != nil {
		t.Fatalf("DecodeState: %v", err)
	}
	return state
}

func TestDecodeBoard(t *testing.T) {
	state := mustDecode(t, newBoardReader(
		"#######",
		"#     #",
		"# @$. #",
		"#  $  #",
		"#  .  #",
		"#     #",
		"#######",
	))
	if state.Screen != ScreenPuzzle {
		t.Fatalf("Screen = %s, want puzzle", state.Screen)
	}
	if state.Width != 7 || state.Height != 7 {
		t.Fatalf("size = %dx%d, want 7x7", state.Width, state.Height)
	}
	if state.Player == nil || *state.Player != (Pos{2, 2}) {
		t.Fatalf("Player = %v, want (2,2)", state.Player)
	}
	if want := []Pos{{3, 2}, {3, 3}}; !samePosSet(state.Crates, want) {
		t.Fatalf("Crates = %v, want %v", state.Crates, want)
	}
	if want := []Pos{{4, 2}, {3, 4}}; !samePosSet(state.Goals, want) {
		t.Fatalf("Goals = %v, want %v", state.Goals, want)
	}
	if state.Solved {
		t.Fatal("board with loose crates reported solved")
	}
}

// Floor and the area outside the walls are drawn identically, so the decoder
// must seal what the player cannot reach; otherwise a crate could be planned
// into the exterior.
func TestDecodeSealsExterior(t *testing.T) {
	state := mustDecode(t, newBoardReader(
		"#####  ",
		"#@$.# ##",
		"#####  ",
	))
	for _, outside := range []Pos{{5, 0}, {6, 0}, {5, 2}} {
		if !containsPos(state.Walls, outside) {
			t.Errorf("exterior cell %v not sealed as wall; walls = %v", outside, state.Walls)
		}
	}
	for _, inside := range []Pos{{1, 1}, {2, 1}, {3, 1}} {
		if containsPos(state.Walls, inside) {
			t.Errorf("interior cell %v wrongly sealed", inside)
		}
	}
}

// A pushed crate is a sprite, not a background cell, and it stays one.
func TestDecodeSpriteCrate(t *testing.T) {
	f := newBoardReader(
		"########",
		"#@    .#",
		"########",
		"########",
	)
	f.putSpriteCrate(3, 1, sym.TileCrate)
	state := mustDecode(t, f)
	if want := []Pos{{3, 1}}; !samePosSet(state.Crates, want) {
		t.Fatalf("Crates = %v, want %v", state.Crates, want)
	}
	if state.Solved {
		t.Fatal("crate off its goal reported solved")
	}
}

func TestDecodeSolved(t *testing.T) {
	f := newBoardReader(
		"########",
		"#@    .#",
		"########",
		"########",
	)
	f.putSpriteCrate(6, 1, sym.TileCrateOnGoal)
	state := mustDecode(t, f)
	if !state.Solved || state.Screen != ScreenSolved {
		t.Fatalf("Solved=%v Screen=%s, want solved", state.Solved, state.Screen)
	}
	if len(state.Goals) != 1 || len(state.Crates) != 1 {
		t.Fatalf("goals=%v crates=%v, want one of each (no duplicate goal)", state.Goals, state.Crates)
	}
}

func TestDecodeBackgroundCrateOnGoal(t *testing.T) {
	state := mustDecode(t, newBoardReader(
		"#####",
		"#@ *#",
		"#####",
		"#####",
	))
	if !state.Solved {
		t.Fatalf("crate on goal not solved: crates=%v goals=%v", state.Crates, state.Goals)
	}
}

// Screens that reuse the background map (title, menu, cutscene) have no wall
// grid and must not decode as a puzzle.
func TestDecodeNonBoardScreens(t *testing.T) {
	f := newFakeReader()
	f.mem[sym.LCDC] = 0x91
	for i := 0; i < sym.BGMapSize*sym.BGMapSize; i++ {
		f.mem[int(sym.BGMapLow)+i] = byte(i) // text-like noise, no wall cells
	}
	state := mustDecode(t, f)
	if state.Screen != ScreenUnknown || state.Player != nil {
		t.Fatalf("non-board decoded as %s player=%v", state.Screen, state.Player)
	}
}

// Wall cells that do not share one 2x2 grid are not a board the decoder
// understands; that is an error, not a guess.
func TestDecodeMisaligned(t *testing.T) {
	f := newBoardReader(
		"#####",
		"#@ $#",
		"#####",
		"#####",
	)
	f.putStray(sym.TileWall, 20, 20) // odd offset from the board's grid
	if _, err := DecodeState(f); !errors.Is(err, ErrMisaligned) {
		t.Fatalf("err = %v, want ErrMisaligned", err)
	}
}

func (f *fakeReader) putStray(tl byte, col, row int) {
	base := int(sym.BGMapLow)
	f.mem[base+row*sym.BGMapSize+col] = tl
	f.mem[base+row*sym.BGMapSize+col+1] = tl + 1
	f.mem[base+(row+1)*sym.BGMapSize+col] = tl + 2
	f.mem[base+(row+1)*sym.BGMapSize+col+1] = tl + 3
}

// Mid-step the sprite sits between cells: report no player rather than a
// wrong cell, so the controller waits instead of acting on a bad position.
func TestDecodePlayerBetweenCells(t *testing.T) {
	f := newBoardReader(
		"#####",
		"#@ $#",
		"#####",
		"#####",
	)
	f.mem[sym.PlayerSprite] += 5
	state := mustDecode(t, f)
	if state.Player != nil {
		t.Fatalf("Player = %v mid-step, want nil", state.Player)
	}
}

func TestDecodeHonoursScroll(t *testing.T) {
	f := newBoardReader(
		"#####",
		"#@ $#",
		"#####",
		"#####",
	)
	// Scrolling the view 16px right slides the map left under the screen, so a
	// sprite that stays put on screen is now one cell further along the map.
	f.mem[sym.SCX] = 16
	state := mustDecode(t, f)
	if state.Player == nil || *state.Player != (Pos{2, 1}) {
		t.Fatalf("Player = %v, want (2,1) after scroll", state.Player)
	}
}

func TestDecodeReadsSelectedBackgroundMap(t *testing.T) {
	f := newBoardReader(
		"#####",
		"#@ $#",
		"#####",
		"#####",
	)
	// Move the board to the high map and select it; the low map is blank.
	for i := 0; i < sym.BGMapSize*sym.BGMapSize; i++ {
		f.mem[int(sym.BGMapHigh)+i] = f.mem[int(sym.BGMapLow)+i]
		f.mem[int(sym.BGMapLow)+i] = sym.TileEmpty
	}
	if st := mustDecode(t, f); st.Screen != ScreenUnknown {
		t.Fatalf("low map selected but blank decoded as %s", st.Screen)
	}
	f.mem[sym.LCDC] |= sym.LCDCBGMapHigh
	if st := mustDecode(t, f); st.Screen != ScreenPuzzle {
		t.Fatalf("high map selected decoded as %s, want puzzle", st.Screen)
	}
}

func TestDecodeNilReader(t *testing.T) {
	if _, err := DecodeState(nil); err == nil {
		t.Fatal("DecodeState(nil) = nil error")
	}
}

// TestObserveNilProfile verifies Observe returns an error for a nil profile.
func TestObserveNilProfile(t *testing.T) {
	if _, err := Observe(nil, newFakeReader()); err == nil {
		t.Fatal("Observe(nil) = nil error, want an error")
	}
}

func containsPos(ps []Pos, p Pos) bool {
	for _, q := range ps {
		if q == p {
			return true
		}
	}
	return false
}

// samePosSet reports whether two []Pos contain the same positions (order-
// insensitive).
func samePosSet(a, b []Pos) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[Pos]bool, len(a))
	for _, p := range a {
		am[p] = true
	}
	for _, p := range b {
		if !am[p] {
			return false
		}
	}
	return true
}

// Boards too large for 16x16 cells are drawn one tile per cell with a
// one-tile player and crate sprites (the sixth puzzle on). Stale tiles below
// the visible window must not join the board.
func TestDecodeSmallScaleBoard(t *testing.T) {
	f := newFakeReader()
	f.mem[sym.LCDC] = 0x91
	base := int(sym.BGMapLow)
	for i := 0; i < sym.BGMapSize*sym.BGMapSize; i++ {
		f.mem[base+i] = sym.TileEmpty
	}
	rows := []string{
		"######",
		"#  . #",
		"# $  #",
		"#  * #",
		"######",
	}
	const col0, row0 = 4, 3
	for y, row := range rows {
		for x, ch := range row {
			tile := map[rune]byte{'#': sym.SmallTileWall, '$': sym.SmallTileCrate, '.': sym.SmallTileGoal, '*': sym.SmallTileCrateOnGoal}[ch]
			if tile != 0 {
				f.mem[base+(row0+y)*sym.BGMapSize+col0+x] = tile
			}
		}
	}
	for x := 0; x < 12; x++ { // stale off-screen walls
		f.mem[base+25*sym.BGMapSize+x] = sym.SmallTileWall
	}
	// Player one-tile sprite at cell (1,1); a pushed crate sprite at (4,2).
	f.mem[sym.PlayerSprite], f.mem[sym.PlayerSprite+1], f.mem[sym.PlayerSprite+2] = byte(16+8*(row0+1)), byte(8+8*(col0+1)), 0xB0
	crate := sym.OAMBase + sym.OAMEntry
	f.mem[crate], f.mem[crate+1], f.mem[crate+2] = byte(16+8*(row0+2)), byte(8+8*(col0+4)), sym.SmallTileCrate

	state := mustDecode(t, f)
	if state.Screen != ScreenPuzzle || state.Width != 6 || state.Height != 5 {
		t.Fatalf("decoded %s %dx%d, want puzzle 6x5", state.Screen, state.Width, state.Height)
	}
	if state.Player == nil || *state.Player != (Pos{X: 1, Y: 1}) {
		t.Fatalf("player = %v, want (1,1)", state.Player)
	}
	if len(state.Crates) != 3 || len(state.Goals) != 2 {
		t.Fatalf("crates=%v goals=%v, want 3 crates and 2 goals", state.Crates, state.Goals)
	}
}
