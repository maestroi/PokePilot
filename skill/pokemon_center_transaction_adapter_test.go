package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

type fakeCenterPhase uint8

const (
	fakeCenterOverworld fakeCenterPhase = iota
	fakeCenterWelcome
	fakeCenterPrompt
	fakeCenterHealing
	fakeCenterFarewell
	fakeCenterDone
)

type fakeGen2CenterMachine struct {
	phase      fakeCenterPhase
	healFrames int
	recovered  bool
	cursor     int
}

func (*fakeGen2CenterMachine) Peek8(uint16) byte       { return 0 }
func (*fakeGen2CenterMachine) PeekInto(uint16, []byte) {}

func (m *fakeGen2CenterMachine) StepFrame() {
	if m.phase == fakeCenterHealing {
		m.healFrames++
		if m.healFrames >= 3 {
			m.recovered = true
			m.phase = fakeCenterFarewell
		}
	}
}

func (m *fakeGen2CenterMachine) StepFrames(n int) {
	for i := 0; i < n; i++ {
		m.StepFrame()
	}
}

func (m *fakeGen2CenterMachine) Tap(btn emu.Button, _, _ int) {
	if btn == emu.Down && m.phase == fakeCenterPrompt {
		m.cursor = 1
		return
	}
	if btn == emu.Up && m.phase == fakeCenterPrompt {
		m.cursor = 0
		return
	}
	if btn != emu.A {
		return
	}
	switch m.phase {
	case fakeCenterOverworld:
		m.phase = fakeCenterWelcome
	case fakeCenterWelcome:
		m.phase = fakeCenterPrompt
	case fakeCenterPrompt:
		if m.cursor == 0 {
			m.phase = fakeCenterHealing
		} else {
			m.phase = fakeCenterFarewell
		}
	case fakeCenterFarewell:
		m.phase = fakeCenterDone
	}
}

type fakeGen2CenterRuntime struct{}

func (fakeGen2CenterRuntime) DecodeCenter(r game.MemoryReader) game.CenterState {
	m := r.(*fakeGen2CenterMachine)
	return game.CenterState{
		PartyPresent: true,
		Recovered:    m.recovered,
		TextOpen:     m.phase == fakeCenterWelcome || m.phase == fakeCenterFarewell,
		PromptOpen:   m.phase == fakeCenterPrompt,
		MenuOpen:     m.phase == fakeCenterPrompt,
	}
}

func (fakeGen2CenterRuntime) DecodeOverworld(r game.MemoryReader) game.OverworldState {
	m := r.(*fakeGen2CenterMachine)
	return game.OverworldState{
		NativeMapID:  0x0112,
		X:            9,
		Y:            6,
		Controllable: m.phase == fakeCenterOverworld || m.phase == fakeCenterDone,
	}
}

func (fakeGen2CenterRuntime) DecodeMenuCursor(r game.MemoryReader) game.MenuCursorState {
	m := r.(*fakeGen2CenterMachine)
	return game.MenuCursorState{Current: m.cursor, Max: 1}
}

func (fakeGen2CenterRuntime) DecodeTwoOption(r game.MemoryReader) (game.TwoOptionState, bool) {
	m := r.(*fakeGen2CenterMachine)
	if m.phase != fakeCenterPrompt {
		return game.TwoOptionState{}, false
	}
	return game.TwoOptionState{Current: m.cursor}, true
}

func (fakeGen2CenterRuntime) DecodeStartMenu(game.MemoryReader) game.StartMenuState {
	return game.StartMenuState{}
}

func (fakeGen2CenterRuntime) StartMenuEntryIndex(game.MemoryReader, game.StartMenuEntry) (int, bool) {
	return 0, false
}

func TestPokemonCenterTransactionUsesFakeGen2SemanticState(t *testing.T) {
	m := &fakeGen2CenterMachine{}
	if err := healAtNurse(m, fakeGen2CenterRuntime{}); err != nil {
		t.Fatalf("fake Gen-II Center transaction: %v", err)
	}
	if !m.recovered {
		t.Fatal("Center transaction returned without recovery")
	}
	if m.phase != fakeCenterDone {
		t.Fatalf("phase = %d, want stable overworld completion", m.phase)
	}
}
