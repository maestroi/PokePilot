package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const (
	fakeSwitchStage uint16 = 144 + iota
	fakeSwitchBattleColumn
	fakeSwitchBattleRow
	fakeSwitchPartyCursor
	fakeSwitchActiveSlot
	fakeSwitchSelectedSlot
)

const (
	fakeSwitchStageBattle byte = iota
	fakeSwitchStageParty
	fakeSwitchStageConfirm
)

type fakeGen2SwitchBattleMenu struct{}

func (fakeGen2SwitchBattleMenu) DecodeBattleMainMenu(r game.MemoryReader) game.BattleMainMenuState {
	if r.Peek8(fakeSwitchStage) != fakeSwitchStageBattle {
		return game.BattleMainMenuState{}
	}
	return game.BattleMainMenuState{
		Visible: true,
		Cursor: game.BattleMenuPosition{
			Column: int(r.Peek8(fakeSwitchBattleColumn)),
			Row:    int(r.Peek8(fakeSwitchBattleRow)),
		},
	}
}

func (fakeGen2SwitchBattleMenu) BattleMainMenuEntryPosition(entry game.BattleMenuEntry) (game.BattleMenuPosition, bool) {
	switch entry {
	case game.BattleMenuPokemon:
		return game.BattleMenuPosition{Column: 2, Row: 1}, true
	case game.BattleMenuFight:
		return game.BattleMenuPosition{Column: 0, Row: 0}, true
	case game.BattleMenuItems:
		return game.BattleMenuPosition{Column: 1, Row: 2}, true
	case game.BattleMenuRun:
		return game.BattleMenuPosition{Column: 2, Row: 2}, true
	default:
		return game.BattleMenuPosition{}, false
	}
}

type fakeGen2SwitchPartyMenu struct{}

func (fakeGen2SwitchPartyMenu) DecodePartyMenu(r game.MemoryReader) game.PartyMenuState {
	if r.Peek8(fakeSwitchStage) != fakeSwitchStageParty {
		return game.PartyMenuState{}
	}
	return game.PartyMenuState{
		Visible: true,
		Kind:    game.PartyMenuVoluntaryBattle,
		Cursor: game.MenuCursorState{
			Current: int(r.Peek8(fakeSwitchPartyCursor)),
			Max:     2,
		},
	}
}

type fakeGen2SwitchExecution struct{}

func (fakeGen2SwitchExecution) DecodeBattleExecution(r game.MemoryReader) game.BattleExecutionState {
	phase := game.BattleExecutionNone
	switch r.Peek8(fakeSwitchStage) {
	case fakeSwitchStageBattle:
		phase = game.BattleExecutionMainMenu
	case fakeSwitchStageConfirm:
		phase = game.BattleExecutionSwitchBox
	}
	return game.BattleExecutionState{InBattle: true, Phase: phase}
}

type fakeGen2SwitchResources struct {
	faintedSlot int
}

func (d fakeGen2SwitchResources) DecodeBattleResources(r game.MemoryReader) game.BattleResourcesState {
	party := []game.BattlePartyMon{
		{NativeSpeciesID: 152, HP: 30, MaxHP: 30},
		{NativeSpeciesID: 251, HP: 40, MaxHP: 40},
		{NativeSpeciesID: 249, HP: 55, MaxHP: 55},
	}
	if d.faintedSlot >= 0 && d.faintedSlot < len(party) {
		party[d.faintedSlot].HP = 0
	}
	return game.BattleResourcesState{
		InBattle:   true,
		ActiveSlot: int(r.Peek8(fakeSwitchActiveSlot)),
		Party:      party,
	}
}

type fakeSwitchMachine struct {
	mem [192]byte
}

func (m *fakeSwitchMachine) Peek8(addr uint16) byte { return m.mem[addr] }

func (m *fakeSwitchMachine) PeekInto(addr uint16, dst []byte) {
	copy(dst, m.mem[int(addr):])
}

func (*fakeSwitchMachine) StepFrame()      {}
func (*fakeSwitchMachine) StepFrames(int) {}

func (m *fakeSwitchMachine) Tap(btn emu.Button, _, _ int) {
	switch m.mem[fakeSwitchStage] {
	case fakeSwitchStageBattle:
		switch btn {
		case emu.Left:
			if m.mem[fakeSwitchBattleColumn] > 0 {
				m.mem[fakeSwitchBattleColumn]--
			}
		case emu.Right:
			if m.mem[fakeSwitchBattleColumn] < 2 {
				m.mem[fakeSwitchBattleColumn]++
			}
		case emu.Up:
			if m.mem[fakeSwitchBattleRow] > 0 {
				m.mem[fakeSwitchBattleRow]--
			}
		case emu.Down:
			if m.mem[fakeSwitchBattleRow] < 2 {
				m.mem[fakeSwitchBattleRow]++
			}
		case emu.A:
			if m.mem[fakeSwitchBattleColumn] == 2 && m.mem[fakeSwitchBattleRow] == 1 {
				m.mem[fakeSwitchStage] = fakeSwitchStageParty
			}
		}
	case fakeSwitchStageParty:
		switch btn {
		case emu.Up:
			if m.mem[fakeSwitchPartyCursor] > 0 {
				m.mem[fakeSwitchPartyCursor]--
			}
		case emu.Down:
			if m.mem[fakeSwitchPartyCursor] < 2 {
				m.mem[fakeSwitchPartyCursor]++
			}
		case emu.A:
			m.mem[fakeSwitchSelectedSlot] = m.mem[fakeSwitchPartyCursor]
			m.mem[fakeSwitchStage] = fakeSwitchStageConfirm
		}
	case fakeSwitchStageConfirm:
		if btn == emu.A {
			m.mem[fakeSwitchActiveSlot] = m.mem[fakeSwitchSelectedSlot]
			m.mem[fakeSwitchStage] = fakeSwitchStageBattle
		}
	}
}

func TestSwitchActiveTransactionUsesGen2ShapedProfileState(t *testing.T) {
	m := &fakeSwitchMachine{}
	m.mem[fakeSwitchStage] = fakeSwitchStageBattle
	m.mem[fakeSwitchActiveSlot] = 0

	err := switchActiveWithDecoders(
		m,
		2,
		fakeGen2SwitchBattleMenu{},
		fakeGen2SwitchPartyMenu{},
		fakeGen2SwitchExecution{},
		fakeGen2SwitchResources{faintedSlot: -1},
	)
	if err != nil {
		t.Fatalf("switch active: %v", err)
	}
	if got := int(m.mem[fakeSwitchActiveSlot]); got != 2 {
		t.Fatalf("active slot = %d, want 2", got)
	}
}

func TestSwitchActiveRejectsFaintedTargetBeforeInput(t *testing.T) {
	m := &fakeSwitchMachine{}
	m.mem[fakeSwitchStage] = fakeSwitchStageBattle
	m.mem[fakeSwitchActiveSlot] = 0

	err := switchActiveWithDecoders(
		m,
		1,
		fakeGen2SwitchBattleMenu{},
		fakeGen2SwitchPartyMenu{},
		fakeGen2SwitchExecution{},
		fakeGen2SwitchResources{faintedSlot: 1},
	)
	if err == nil {
		t.Fatal("fainted switch target was accepted")
	}
	if m.mem[fakeSwitchStage] != fakeSwitchStageBattle ||
		m.mem[fakeSwitchBattleColumn] != 0 ||
		m.mem[fakeSwitchBattleRow] != 0 {
		t.Fatal("rejected switch target changed battle UI state")
	}
}
