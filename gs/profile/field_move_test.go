package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestDecodeGoldStartMenuUsesDynamicRetailList(t *testing.T) {
	var mem fakeMemory
	mem[sym.MenuItemsList] = 8
	copy(mem[sym.MenuItemsList+1:], []byte{0, 1, 2, 7, 3, 4, 5, 6})
	mem[sym.MenuCursorPosition] = 2
	putGSText(&mem, "POKEDEX POKEMON PACK POKEGEAR STATUS SAVE OPTION EXIT")

	state := NewGold().DecodeStartMenu(&mem)
	if !state.Visible || !state.Ready || state.Cursor.Current != 1 || state.Cursor.Max != 7 {
		t.Fatalf("start menu=%+v", state)
	}
	if got, ok := NewGold().StartMenuEntryIndex(&mem, game.StartMenuPokemon); !ok || got != 1 {
		t.Fatalf("pokemon index=%d ok=%v, want 1,true", got, ok)
	}
	if got, ok := NewGold().StartMenuEntryIndex(&mem, game.StartMenuItems); !ok || got != 2 {
		t.Fatalf("pack index=%d ok=%v, want 2,true", got, ok)
	}
	if got := NewGold().DecodeMenuCursor(&mem); got.Current != 1 || got.Max != 7 {
		t.Fatalf("generic start cursor=%+v", got)
	}
}

func TestDecodeGoldCutCapabilityFailsClosedBeforeGen2Teacher(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1] = 0x9b // Cyndaquil; pinned base stats permit Cut.
	mem[sym.JohtoBadges] = 1 << 1
	mem[sym.TMsHMs+50] = 1 // HM01

	capability, ok, err := NewGold().DecodeFieldMoveCapability(&mem, nil, game.FieldMoveCut)
	if err != nil || !ok {
		t.Fatalf("capability ok=%v err=%v", ok, err)
	}
	if !capability.BadgeOwned || !capability.MachineOwned || capability.Learned || capability.Usable {
		t.Fatalf("pre-teach capability=%+v", capability)
	}
	if capability.Preparable {
		t.Fatalf("generic Gen-I teacher must remain gated off for Gen-II: %+v", capability)
	}
	if len(capability.CompatiblePartySlots) != 1 || capability.CompatiblePartySlots[0] != 0 {
		t.Fatalf("compatible slots=%v, want [0]", capability.CompatiblePartySlots)
	}

	mem[sym.PartyMon1+gen2PartyMovesOffset] = byte(gen2MoveCut)
	capability, ok, err = NewGold().DecodeFieldMoveCapability(&mem, nil, game.FieldMoveCut)
	if err != nil || !ok || !capability.Learned || !capability.Usable || capability.PartySlot != 0 {
		t.Fatalf("learned capability=%+v ok=%v err=%v", capability, ok, err)
	}
}

func TestDecodeGoldFieldMoveMenuPreservesMoveOrder(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 1
	mem[sym.CurPartyMon] = 0
	mem[sym.PartyMon1+gen2PartyMovesOffset+0] = byte(gen2MoveHeadbutt)
	mem[sym.PartyMon1+gen2PartyMovesOffset+1] = byte(gen2MoveCut)
	putGSText(&mem, "HEADBUTT CUT STATS SWITCH MOVE ITEM CANCEL")

	got := NewGold().DecodeFieldMoveMenu(&mem)
	if len(got.Entries) != 2 || got.Entries[0] != game.FieldMoveHeadbutt || got.Entries[1] != game.FieldMoveCut {
		t.Fatalf("field move menu=%v", got.Entries)
	}
}

func TestDecodeGoldCutActionState(t *testing.T) {
	var mem fakeMemory
	mem[sym.FacingTileID] = gen2CollisionCutTreeA
	mem[sym.BattlePlayerAction] = 1
	mem[sym.StateFlags] = gen2TextStateMask
	putGSText(&mem, "CYNDAQUIL used CUT!")

	got := NewGold().DecodeFieldAction(&mem)
	if !got.CuttableAhead || !got.ActionSucceeded || !got.ResultTextActive {
		t.Fatalf("field action=%+v", got)
	}
}

func TestDecodeGoldTMHMPocket(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurPocket] = gen2TMHMPocket
	mem[sym.CurItem] = 51
	got := NewGold().DecodeTMHMPocket(&mem)
	if !got.Visible || got.CurrentMachine != 51 {
		t.Fatalf("tmhm pocket=%+v", got)
	}
}
