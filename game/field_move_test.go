package game

import "testing"

func TestProgressionFieldMovesIncludeGenIISemantics(t *testing.T) {
	got := ProgressionFieldMoves()
	want := []FieldMoveID{
		FieldMoveCut,
		FieldMoveFly,
		FieldMoveSurf,
		FieldMoveStrength,
		FieldMoveFlash,
		FieldMoveWhirlpool,
		FieldMoveWaterfall,
		FieldMoveHeadbutt,
	}
	if len(got) != len(want) {
		t.Fatalf("field move vocabulary len=%d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("field move vocabulary[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestFieldMoveCapabilityCanRepresentGenIITMFieldMove(t *testing.T) {
	capability := FieldMoveCapability{
		Move:                 FieldMoveHeadbutt,
		Name:                 "HEADBUTT",
		BadgeOwned:           true, // no badge required
		MachineOwned:         true, // TM02 in Gen II
		PartySlot:            -1,
		CompatiblePartySlots: []int{1, 2},
		Preparable:           true,
	}
	if capability.BadgeRequired != "" {
		t.Fatalf("Headbutt badge requirement=%q, want none", capability.BadgeRequired)
	}
	if !capability.Preparable || !capability.MachineOwned || len(capability.CompatiblePartySlots) != 2 {
		t.Fatalf("Headbutt capability cannot represent Gen-II preparation: %+v", capability)
	}
}
