package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

type numberedMachineTestDecoder struct{}

func (numberedMachineTestDecoder) DecodeFieldMoveCapability(game.MemoryReader, []byte, game.FieldMoveID) (game.FieldMoveCapability, bool, error) {
	return game.FieldMoveCapability{}, true, nil
}
func (numberedMachineTestDecoder) DecodeFieldMoveMenu(game.MemoryReader) game.FieldMoveMenuState {
	return game.FieldMoveMenuState{}
}
func (numberedMachineTestDecoder) NativeFieldMove(id game.FieldMoveID) (game.NativeFieldMove, bool) {
	switch id {
	case game.FieldMoveCut:
		return game.NativeFieldMove{MoveID: 0x0f, MachineNumber: 51}, true
	case game.FieldMoveFly:
		return game.NativeFieldMove{MoveID: 0x13, MachineNumber: 52}, true
	case game.FieldMoveSurf:
		return game.NativeFieldMove{MoveID: 0x39, MachineNumber: 53}, true
	case game.FieldMoveStrength:
		return game.NativeFieldMove{MoveID: 0x46, MachineNumber: 54}, true
	case game.FieldMoveFlash:
		return game.NativeFieldMove{MoveID: 0x94, MachineNumber: 55}, true
	case game.FieldMoveWhirlpool:
		return game.NativeFieldMove{MoveID: 0xfa, MachineNumber: 56}, true
	case game.FieldMoveWaterfall:
		return game.NativeFieldMove{MoveID: 0x7f, MachineNumber: 57}, true
	case game.FieldMoveHeadbutt:
		return game.NativeFieldMove{MoveID: 0x1d, MachineNumber: 2}, true
	default:
		return game.NativeFieldMove{}, false
	}
}

func TestNumberedMachineRecipientPrefersBenchEmptySlot(t *testing.T) {
	capability := game.FieldMoveCapability{CompatiblePartySlots: []int{0, 1}}
	execution := game.BattleExecutionState{PartyMoves: [][4]uint16{
		{33, 45, 52, 0},
		{33, 45, 0, 0},
	}}

	slot, replace, err := numberedMachineRecipient(numberedMachineTestDecoder{}, capability, execution)
	if err != nil {
		t.Fatalf("numberedMachineRecipient: %v", err)
	}
	if slot != 1 || replace != 2 {
		t.Fatalf("recipient = slot %d move %d, want bench slot 1 empty move 2", slot, replace)
	}
}

func TestNumberedMachineRecipientNeverReplacesHM(t *testing.T) {
	capability := game.FieldMoveCapability{CompatiblePartySlots: []int{1}}
	execution := game.BattleExecutionState{PartyMoves: [][4]uint16{
		{},
		{0x0f, 0x13, 0x39, 0x46},
	}}
	if _, _, err := numberedMachineRecipient(numberedMachineTestDecoder{}, capability, execution); err == nil {
		t.Fatal("four-HM carrier unexpectedly considered replaceable")
	}

	execution.PartyMoves[1] = [4]uint16{0x0f, 33, 0x39, 0x46}
	slot, replace, err := numberedMachineRecipient(numberedMachineTestDecoder{}, capability, execution)
	if err != nil {
		t.Fatalf("recipient with one replaceable move: %v", err)
	}
	if slot != 1 || replace != 1 {
		t.Fatalf("recipient = slot %d move %d, want slot 1 replacement 1", slot, replace)
	}
}
