package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
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
		Controllable:       flags&fakeFieldControllable != 0,
		CutTargetKnown:     true,
		CuttableAhead:      flags&fakeFieldCuttable != 0,
		BoulderTargetKnown: true,
		BoulderAhead:       flags&fakeFieldBoulder != 0,
		Surfing:            flags&fakeFieldSurfing != 0,
		StrengthActive:     flags&fakeFieldStrength != 0,
		Lit:                flags&fakeFieldLit != 0,
		ActionSucceeded:    flags&fakeFieldSucceeded != 0,
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
	if err := validateFieldActionRuntime(game.FieldActionState{CutTargetKnown: true}, cut); err == nil {
		t.Fatal("Cut with known-absent semantic target unexpectedly validated")
	}
	if err := validateFieldActionRuntime(game.FieldActionState{}, cut); err != nil {
		t.Fatalf("Cut with profile-unknown target should defer to cartridge: %v", err)
	}
	if err := validateFieldActionRuntime(game.FieldActionState{CutTargetKnown: true, CuttableAhead: true}, cut); err != nil {
		t.Fatalf("Cut target rejected: %v", err)
	}

	strength, _ := FieldMoveSpecFor(FieldStrength)
	if err := validateFieldActionRuntime(game.FieldActionState{BoulderTargetKnown: true, BoulderAhead: true}, strength); err != nil {
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

func TestTraversalModeUsesSemanticSurfState(t *testing.T) {
	if got := traversalModeForFieldActionState(game.FieldActionState{}); got != world.TraversalLand {
		t.Fatalf("non-surfing traversal mode = %v, want land", got)
	}
	if got := traversalModeForFieldActionState(game.FieldActionState{Surfing: true}); got != world.TraversalWater {
		t.Fatalf("surfing traversal mode = %v, want water", got)
	}
}

// refusedFieldMoveMachine models the ROM returning to the field-move party
// list after refusing a field move ("No SURFing ... here!"). B closes it.
type refusedFieldMoveMachine struct {
	menuOpen bool
	kind     game.PartyMenuKind
}

func (m *refusedFieldMoveMachine) Peek8(uint16) byte          { return 0 }
func (m *refusedFieldMoveMachine) PeekInto(uint16, []byte)    {}
func (m *refusedFieldMoveMachine) StepFrame()                 {}
func (m *refusedFieldMoveMachine) StepFrames(int)             {}
func (m *refusedFieldMoveMachine) Tap(b emu.Button, _, _ int) { m.menuOpen = m.menuOpen && b != emu.B }
func (m *refusedFieldMoveMachine) DecodeFieldAction(game.MemoryReader) game.FieldActionState {
	return game.FieldActionState{Controllable: !m.menuOpen, ChoiceVisible: m.menuOpen}
}
func (m *refusedFieldMoveMachine) DecodePartyMenu(game.MemoryReader) game.PartyMenuState {
	return game.PartyMenuState{Visible: m.menuOpen, Kind: m.kind}
}

func TestCloseFieldActionCancelsOwnFieldMovePartyMenu(t *testing.T) {
	m := &refusedFieldMoveMachine{menuOpen: true, kind: game.PartyMenuFieldMove}
	if err := closeFieldActionToOverworld(m, m, m); err != nil || m.menuOpen {
		t.Fatalf("close = %v, menuOpen=%v; want the refused field-move party list cancelled", err, m.menuOpen)
	}

	foreign := &refusedFieldMoveMachine{menuOpen: true, kind: game.PartyMenuForcedBattle}
	if err := closeFieldActionToOverworld(foreign, foreign, foreign); err == nil || !foreign.menuOpen {
		t.Fatalf("close = %v, menuOpen=%v; a foreign choice must not be answered", err, foreign.menuOpen)
	}
}

func TestObstacleFieldMovesCompleteOnROMSuccessByte(t *testing.T) {
	for _, move := range []FieldMove{FieldCut, FieldRockSmash, FieldWhirlpool} {
		spec := FieldMoveSpec{Move: move, Name: move.String()}
		if fieldActionCompleteState(game.FieldActionState{Controllable: true}, spec) {
			t.Fatalf("%s completed without the ROM success byte", move)
		}
		if !fieldActionCompleteState(game.FieldActionState{Controllable: true, ActionSucceeded: true}, spec) {
			t.Fatalf("%s did not complete after the ROM success byte", move)
		}
	}
}

type battleFieldActionMachine struct{ taps int }

func (m *battleFieldActionMachine) Peek8(uint16) byte        { return 0 }
func (m *battleFieldActionMachine) PeekInto(uint16, []byte)  {}
func (m *battleFieldActionMachine) StepFrame()               {}
func (m *battleFieldActionMachine) StepFrames(int)           {}
func (m *battleFieldActionMachine) Tap(emu.Button, int, int) { m.taps++ }
func (m *battleFieldActionMachine) DecodeFieldAction(game.MemoryReader) game.FieldActionState {
	return game.FieldActionState{ActionSucceeded: true, ResultTextActive: true, InBattle: true}
}

// Rock Smash can start a wild battle. The settle loop must surface that as
// ErrBattle without paging the battle's text boxes with A.
func TestSettleFieldActionYieldsToBattle(t *testing.T) {
	m := &battleFieldActionMachine{}
	err := settleFieldAction(m, FieldMoveSpec{Move: FieldRockSmash, Name: "ROCK SMASH"}, m)
	if !errors.Is(err, ErrBattle) || m.taps != 0 {
		t.Fatalf("settle = %v with %d taps, want ErrBattle and no taps", err, m.taps)
	}
}
