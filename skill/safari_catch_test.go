package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestSafariBallNextInputUsesLiveMenuState(t *testing.T) {
	mem := newFakeRAM()
	openTextBox(mem, "BALL BAIT THROW ROCK RUN")

	mem[sym.TopMenuItemX] = safariBattleMenuRightX
	mem[sym.CurrentMenuItem] = mainMenuMax
	if btn, done := safariBallNextInput(mem); done || btn != emu.Up {
		t.Fatalf("RUN -> (%v,%v), want Up,false", btn, done)
	}

	mem[sym.CurrentMenuItem] = 0
	if btn, done := safariBallNextInput(mem); done || btn != emu.Left {
		t.Fatalf("BAIT -> (%v,%v), want Left,false", btn, done)
	}

	mem[sym.TopMenuItemX] = safariBattleMenuLeftX
	if btn, done := safariBallNextInput(mem); !done || btn != 0 {
		t.Fatalf("BALL -> (%v,%v), want zero,true", btn, done)
	}
	if !safariBallCursor(mem) {
		t.Fatal("Safari BALL cursor should be positively recognized")
	}
}

func TestSafariBallCursorRequiresSafariMenu(t *testing.T) {
	mem := newFakeRAM()
	openTextBox(mem, "FIGHT ITEM PKMN RUN")
	mem[sym.TopMenuItemX] = safariBattleMenuLeftX
	mem[sym.CurrentMenuItem] = 0
	if safariBallCursor(mem) {
		t.Fatal("cursor coordinates alone must not classify a normal battle menu as Safari BALL")
	}
}

func TestIsSafariHabitatMapOnlyAcceptsOutdoorAreas(t *testing.T) {
	for _, mapID := range []uint8{safariZoneEastMap, safariZoneNorthMap, safariZoneWestMap, safariZoneCenterMap} {
		if !isSafariHabitatMap(mapID) {
			t.Fatalf("Safari outdoor map %#04x rejected", mapID)
		}
	}
	for _, mapID := range []uint8{safariZoneGateMap, safariZoneSecretHouse, fuchsiaCityMap} {
		if isSafariHabitatMap(mapID) {
			t.Fatalf("non-habitat map %#04x accepted", mapID)
		}
	}
}
