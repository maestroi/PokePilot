package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

const (
	fakeFieldFlags uint16 = 220
)

const (
	fakeFieldControllable = 1 << iota
	fakeFieldCuttable
	fakeFieldBoulder
	fakeFieldSurfing
	fakeFieldStrength
	fakeFieldLit
	fakeFieldSucceeded
)

type fakeGen2FieldActionDecoder struct{}

func (fakeGen2FieldActionDecoder) DecodeFieldAction(r game.MemoryReader) game.FieldActionState {
	flags := r.Peek8(fakeFieldFlags)
	return game.FieldActionState{
		Controllable:    flags&fakeFieldControllable != 0,
		CuttableAhead:   flags&fakeFieldCuttable != 0,
		BoulderAhead:    flags&fakeFieldBoulder != 0,
		Surfing:         flags&fakeFieldSurfing != 0,
		StrengthActive:  flags&fakeFieldStrength != 0,
		Lit:             flags&fakeFieldLit != 0,
		ActionSucceeded: flags&fakeFieldSucceeded != 0,
	}
}

func TestFieldActionRuntimeAcceptsFakeGen2State(t *testing.T) {
	m := &fakeOverworldMachine{}
	m.mem[fakeFieldFlags] = fakeFieldControllable | fakeFieldSurfing | fakeFieldSucceeded
	state := fakeGen2FieldActionDecoder{}.DecodeFieldAction(m)

	surf, _ := FieldMoveSpecFor(FieldSurf)
	if !fieldActionCompleteState(state, surf) {
		t.Fatalf("Surf state = %+v, want complete", state)
	}
	result := fieldActionResultFromState(FieldSurf, 2, state)
	if !result.Surfing || result.ActionResult != 1 || result.PartySlot != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestFieldActionRuntimeValidatesSemanticTargets(t *testing.T) {
	cut, _ := FieldMoveSpecFor(FieldCut)
	if err := validateFieldActionRuntime(game.FieldActionState{}, cut); err == nil {
		t.Fatal("Cut without semantic target unexpectedly validated")
	}
	if err := validateFieldActionRuntime(game.FieldActionState{CuttableAhead: true}, cut); err != nil {
		t.Fatalf("Cut target rejected: %v", err)
	}

	strength, _ := FieldMoveSpecFor(FieldStrength)
	if err := validateFieldActionRuntime(game.FieldActionState{BoulderAhead: true}, strength); err != nil {
		t.Fatalf("Strength target rejected: %v", err)
	}
}

func TestFieldActionRuntimeUsesSemanticModeFacts(t *testing.T) {
	surf, _ := FieldMoveSpecFor(FieldSurf)
	if err := validateFieldActionRuntime(game.FieldActionState{Surfing: true}, surf); err == nil {
		t.Fatal("already-surfing state unexpectedly validated")
	}

	flash, _ := FieldMoveSpecFor(FieldFlash)
	if err := validateFieldActionRuntime(game.FieldActionState{Lit: true}, flash); err == nil {
		t.Fatal("already-lit state unexpectedly validated")
	}
}
