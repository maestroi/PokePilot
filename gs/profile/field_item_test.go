package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestGSFieldItemProjectsNativePartyAndCorrectPP(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		var m fakeMemory
		m[sym.MapWidth], m[sym.MapHeight] = 10, 10
		m[sym.MapStatus] = gen2MapStatusHandle
		m[sym.PartyCount] = 1
		m[sym.PartyMon1] = 157
		m[sym.PartyMon1+gen2PartyLevelOffset] = 44
		m[sym.PartyMon1+gen2PartyHPOffset] = 0
		m[sym.PartyMon1+gen2PartyHPOffset+1] = 35
		m[sym.PartyMon1+gen2PartyMaxHPOffset] = 0
		m[sym.PartyMon1+gen2PartyMaxHPOffset+1] = 130
		m[sym.PartyMon1+gen2PartyStatusOffset] = 0x08
		m[sym.PartyMon1+gen2PartyMovesOffset] = 53
		m[sym.PartyMon1+gen2PartyPPOffset] = 0xc7 // PP Ups in upper 2 bits
		state := p.DecodeFieldItem(&m)
		if !state.OverworldReady || len(state.Party) != 1 ||
			state.Party[0].HP != 35 || state.Party[0].MaxHP != 130 ||
			state.Party[0].Status != "poisoned" || state.Party[0].PP[0] != 7 {
			t.Fatalf("%s: invalid field party %+v", p.ID(), state)
		}
		if p.FieldItemSemantics(gsMedicineEther) != (game.FieldItemSemantics{SingleMoveTarget: true, PPRestore: true}) {
			t.Fatalf("%s Ether semantics wrong", p.ID())
		}
		if p.FieldItemSemantics(gsMedicineElixer) != (game.FieldItemSemantics{PPRestore: true}) {
			t.Fatalf("%s Elixer semantics wrong", p.ID())
		}
	}
}

func TestGSFieldItemOnlyUsesRenderedPackUI(t *testing.T) {
	p := NewGold()
	var m fakeMemory
	m[sym.MapWidth], m[sym.MapHeight] = 10, 10
	m[sym.MapStatus] = gen2MapStatusHandle
	m[sym.CurPocket] = gen2PocketItems
	m[sym.TwoDMenuNumRows] = 4
	m[sym.TwoDMenuNumCols] = 1
	m[sym.MenuCursorX], m[sym.MenuCursorY] = 1, 1
	// A stale cursor has no menu until the correct words are rendered.
	if got := p.DecodeFieldItem(&m); got.UsePromptVisible || !got.OverworldReady {
		t.Fatalf("stale menu RAM considered active: %+v", got)
	}
	putGSText(&m, "USE GIVE TOSS QUIT")
	got := p.DecodeFieldItem(&m)
	if !got.UsePromptVisible || !got.UseSelected || got.OverworldReady {
		t.Fatalf("native submenu not recognized: %+v", got)
	}
	m[sym.MenuCursorY] = 2
	if got := p.DecodeFieldItem(&m); !got.UsePromptVisible || got.UseSelected {
		t.Fatalf("wrong USE cursor accepted: %+v", got)
	}
	// A pack pocket's rendered graphic blocks the overworld, independent
	// of persistent menu-register state.
	putGSPackGraphic(&m)
	if got := p.DecodeFieldItem(&m); got.OverworldReady {
		t.Fatal("open PACK menu treated as overworld")
	}
}

func TestGSFieldItemRecognizesOverworldPartyUsePrompt(t *testing.T) {
	p := NewSilver()
	var m fakeMemory
	m[sym.PartyCount] = 2
	m[sym.TwoDMenuNumRows] = 3
	m[sym.TwoDMenuNumCols] = 1
	m[sym.MenuCursorX], m[sym.MenuCursorY] = 1, 1
	m[sym.MenuJoypadFilter] = gen2PadA | gen2PadB
	putGSText(&m, "Use on which POKEMON? CANCEL")
	if got := p.DecodePartyMenu(&m); !got.Visible || got.Kind != game.PartyMenuItemUse {
		t.Fatalf("overworld medicine selector not recognized: %+v", got)
	}
	m[sym.TwoDMenuNumRows] = 2
	if got := p.DecodePartyMenu(&m); got.Visible {
		t.Fatal("wrong count/cursor must not be treated as party selector")
	}
}
