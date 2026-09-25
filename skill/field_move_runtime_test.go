package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

type fakeGen2FieldMoveDecoder struct{}

func (fakeGen2FieldMoveDecoder) DecodeFieldMoveCapability(_ game.MemoryReader, _ []byte, id game.FieldMoveID) (game.FieldMoveCapability, bool, error) {
	switch id {
	case game.FieldMoveWhirlpool:
		return game.FieldMoveCapability{
			Move:                 id,
			Name:                 "WHIRLPOOL",
			BadgeRequired:        "Glacier",
			BadgeOwned:           true,
			MachineOwned:         true,
			PartySlot:            -1,
			CompatiblePartySlots: []int{1},
			Preparable:           true,
		}, true, nil
	case game.FieldMoveHeadbutt:
		return game.FieldMoveCapability{
			Move:          id,
			Name:          "HEADBUTT",
			BadgeOwned:    true,
			MachineOwned:  true,
			Learned:       true,
			PartySlot:     2,
			Preparable:    true,
			Usable:        true,
		}, true, nil
	default:
		return game.FieldMoveCapability{}, false, nil
	}
}

func (fakeGen2FieldMoveDecoder) DecodeFieldMoveMenu(game.MemoryReader) game.FieldMoveMenuState {
	// Deliberately include a non-progression action between semantic entries.
	// The empty slot must not shift the native index of Whirlpool/Waterfall.
	return game.FieldMoveMenuState{Entries: []game.FieldMoveID{
		game.FieldMoveHeadbutt,
		"",
		game.FieldMoveWhirlpool,
		game.FieldMoveWaterfall,
	}}
}

func (fakeGen2FieldMoveDecoder) NativeFieldMove(id game.FieldMoveID) (game.NativeFieldMove, bool) {
	switch id {
	case game.FieldMoveWhirlpool:
		return game.NativeFieldMove{MachineItemID: 0x106, MoveID: 0x150}, true
	case game.FieldMoveWaterfall:
		return game.NativeFieldMove{MachineItemID: 0x107, MoveID: 0x151}, true
	case game.FieldMoveHeadbutt:
		return game.NativeFieldMove{MachineItemID: 0x102, MoveID: 0x11d}, true
	default:
		return game.NativeFieldMove{}, false
	}
}

func TestSemanticFieldMovesIncludeGenIISet(t *testing.T) {
	moves := SemanticFieldMoves()
	want := []game.FieldMoveID{
		game.FieldMoveCut,
		game.FieldMoveFly,
		game.FieldMoveSurf,
		game.FieldMoveStrength,
		game.FieldMoveFlash,
		game.FieldMoveWhirlpool,
		game.FieldMoveWaterfall,
		game.FieldMoveHeadbutt,
	}
	if len(moves) != len(want) {
		t.Fatalf("semantic moves len=%d, want %d", len(moves), len(want))
	}
	for i, move := range moves {
		id, ok := semanticFieldMove(move)
		if !ok || id != want[i] {
			t.Fatalf("semantic move[%d]=%v -> %q,%v; want %q", i, move, id, ok, want[i])
		}
	}
}

func TestFieldMoveCapabilityAcceptsFakeGen2CarrierSemantics(t *testing.T) {
	m := &fakeMenuMachine{}
	capability, err := fieldMoveCapabilityWithProfile(fakeGen2FieldMoveDecoder{}, m, nil, FieldWhirlpool)
	if err != nil {
		t.Fatalf("decode fake Gen-II Whirlpool: %v", err)
	}
	if !capability.BadgeOwned || !capability.MachineOwned || !capability.Preparable || capability.Usable {
		t.Fatalf("Whirlpool capability=%+v, want owned/preparable but not yet usable", capability)
	}
	if capability.PartySlot != -1 || len(capability.CompatiblePartySlots) != 1 || capability.CompatiblePartySlots[0] != 1 {
		t.Fatalf("Whirlpool carrier projection=%+v", capability)
	}

	headbutt, err := fieldMoveCapabilityWithProfile(fakeGen2FieldMoveDecoder{}, m, nil, FieldHeadbutt)
	if err != nil {
		t.Fatalf("decode fake Gen-II Headbutt: %v", err)
	}
	if headbutt.BadgeRequired != "" || !headbutt.Usable || headbutt.PartySlot != 2 {
		t.Fatalf("Headbutt capability=%+v, want badge-free learned carrier", headbutt)
	}
}

func TestFieldMoveMenuExecutesFakeGen2SemanticEntry(t *testing.T) {
	m := &fakeMenuMachine{}
	m.mem[fakeMenuCurrent] = 0
	m.mem[fakeMenuMax] = 3

	if err := selectFieldMoveMenuEntryWithDecoders(m, fakeGen2FieldMoveDecoder{}, fakeGen2MenuDecoder{}, FieldWhirlpool); err != nil {
		t.Fatalf("select fake Gen-II Whirlpool: %v", err)
	}
	if got := m.mem[fakeMenuCurrent]; got != 2 {
		t.Fatalf("cursor=%d, want Whirlpool native index 2", got)
	}
	if got := m.mem[fakeSelected]; got != 3 {
		t.Fatalf("selected marker=%d, want native index-2 marker 3", got)
	}
}

func TestFieldMoveMenuPreservesUnknownNativeEntries(t *testing.T) {
	m := &fakeMenuMachine{}
	if got := fieldMoveMenuIndexWithProfile(fakeGen2FieldMoveDecoder{}, m, FieldWhirlpool); got != 2 {
		t.Fatalf("Whirlpool index=%d, want 2", got)
	}
	if got := fieldMoveMenuIndexWithProfile(fakeGen2FieldMoveDecoder{}, m, FieldWaterfall); got != 3 {
		t.Fatalf("Waterfall index=%d, want 3", got)
	}
}
