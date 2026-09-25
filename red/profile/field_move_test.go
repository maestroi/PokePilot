package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeFieldMoveCapabilityKeepsRedPrerequisitesBehindProfile(t *testing.T) {
	var mem fakeMemory
	mem[sym.ObtainedBadges] = 1 << uint8(state.BadgeSoul)
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonMoves] = 0x39 // Surf
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = redrom.HM01Item + 2
	mem[sym.BagItems+1] = 1

	capability, supported, err := New().DecodeFieldMoveCapability(&mem, nil, game.FieldMoveSurf)
	if err != nil {
		t.Fatalf("DecodeFieldMoveCapability(Surf): %v", err)
	}
	if !supported {
		t.Fatal("Red profile did not advertise Surf")
	}
	if capability.Move != game.FieldMoveSurf || capability.Name != "SURF" {
		t.Fatalf("Surf identity=%+v", capability)
	}
	if capability.BadgeRequired != state.BadgeSoul.String() || !capability.BadgeOwned || !capability.MachineOwned {
		t.Fatalf("Surf prerequisites=%+v", capability)
	}
	if !capability.Learned || !capability.Usable || !capability.Preparable || capability.PartySlot != 0 {
		t.Fatalf("Surf carrier=%+v, want usable slot 0", capability)
	}
}

func TestDecodeFieldMoveCapabilityRejectsGenIIMovesOnRed(t *testing.T) {
	var mem fakeMemory
	for _, id := range []game.FieldMoveID{game.FieldMoveWhirlpool, game.FieldMoveWaterfall, game.FieldMoveHeadbutt} {
		if capability, supported, err := New().DecodeFieldMoveCapability(&mem, nil, id); err != nil || supported {
			t.Fatalf("%s on Red = %+v supported=%v err=%v; want unsupported", id, capability, supported, err)
		}
		if _, ok := New().NativeFieldMove(id); ok {
			t.Fatalf("%s unexpectedly has a Red native mapping", id)
		}
	}
}

func TestDecodeFieldMoveMenuPreservesNativePositions(t *testing.T) {
	var mem fakeMemory
	mem[sym.FieldMoves+0] = 1 // Cut
	mem[sym.FieldMoves+1] = 7 // Dig: deliberately outside progression vocabulary
	mem[sym.FieldMoves+2] = 4 // Surf
	mem[sym.FieldMoves+3] = 5 // Strength

	menu := New().DecodeFieldMoveMenu(&mem)
	if len(menu.Entries) != 4 {
		t.Fatalf("field menu len=%d, want 4: %v", len(menu.Entries), menu.Entries)
	}
	want := []game.FieldMoveID{game.FieldMoveCut, "", game.FieldMoveSurf, game.FieldMoveStrength}
	for i := range want {
		if menu.Entries[i] != want[i] {
			t.Fatalf("field menu[%d]=%q, want %q", i, menu.Entries[i], want[i])
		}
	}
}
