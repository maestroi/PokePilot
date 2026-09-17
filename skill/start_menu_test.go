package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

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

func TestStartMenuReadyRejectsStaleMenuRAM(t *testing.T) {
	var mem state.Mem
	mem[sym.MaxMenuItem] = 7
	mem[sym.CurrentMenuItem] = 2
	mem[sym.FontLoaded] = 1

	if startMenuReady(&mem, 7, redWram()) {
		t.Fatal("stale MaxMenuItem/FontLoaded without START-menu labels was accepted")
	}
}

func TestStartMenuReadyRequiresExpectedShapeAndLabels(t *testing.T) {
	var mem state.Mem
	putMenuText(&mem, "SAVE EXIT")
	mem[sym.MaxMenuItem] = 2
	if startMenuReady(&mem, 7, redWram()) {
		t.Fatal("visible labels with stale two-item menu shape were accepted")
	}

	mem[sym.MaxMenuItem] = 7
	if !startMenuReady(&mem, 7, redWram()) {
		t.Fatalf("visible START menu with max=%d was not accepted", mem[sym.MaxMenuItem])
	}
}
