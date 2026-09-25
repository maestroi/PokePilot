package skill

import (
	"testing"

	redprofile "github.com/maestroi/pokepilot/red/profile"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

type startMenuMemReader struct {
	mem *state.Mem
}

func (r startMenuMemReader) Peek8(addr uint16) byte {
	return r.mem[addr]
}

func (r startMenuMemReader) PeekInto(addr uint16, dst []byte) {
	copy(dst, r.mem[int(addr):])
}

func decodeRedStartMenu(mem *state.Mem) bool {
	return redprofile.New().DecodeStartMenu(startMenuMemReader{mem: mem}).Ready
}

func putMenuText(mem *state.Mem, text string) {
	at := int(sym.TileMap)
	for _, r := range text {
		var tile byte
		switch {
		case r == ' ':
			tile = 0x7f
		case r >= 'A' && r <= 'Z':
			tile = 0x80 + byte(r-'A')
		default:
			continue
		}
		(*mem)[at] = tile
		at++
	}
}

func setStartMenuPokedexOwned(mem *state.Mem) {
	event := uint16(state.EventGotPokedex)
	mem[sym.EventFlags+event/8] |= 1 << (event % 8)
}

func TestStartMenuReadyRejectsStaleMenuRAM(t *testing.T) {
	var mem state.Mem
	setStartMenuPokedexOwned(&mem)
	mem[sym.MaxMenuItem] = 7
	mem[sym.CurrentMenuItem] = 2
	mem[sym.FontLoaded] = 1

	if decodeRedStartMenu(&mem) {
		t.Fatal("stale MaxMenuItem/FontLoaded without START-menu labels was accepted")
	}
}

func TestStartMenuReadyRequiresProfileOwnedShapeAndLabels(t *testing.T) {
	var mem state.Mem
	setStartMenuPokedexOwned(&mem)
	putMenuText(&mem, "SAVE EXIT")

	mem[sym.MaxMenuItem] = 2
	if decodeRedStartMenu(&mem) {
		t.Fatal("visible labels with stale two-item menu shape were accepted")
	}

	mem[sym.MaxMenuItem] = 7
	if !decodeRedStartMenu(&mem) {
		t.Fatalf("visible START menu with max=%d was not accepted", mem[sym.MaxMenuItem])
	}
}

func TestStartMenuShapeWithoutPokedexStaysProfileOwned(t *testing.T) {
	var mem state.Mem
	putMenuText(&mem, "SAVE EXIT")
	mem[sym.MaxMenuItem] = 6

	if !decodeRedStartMenu(&mem) {
		t.Fatalf("pre-Pokedex START menu with max=%d was not accepted", mem[sym.MaxMenuItem])
	}
}
