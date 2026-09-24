package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const (
	fakeMenuCurrent uint16 = iota
	fakeMenuMax
	fakePromptOpen
	fakeSelected
	fakeStartVisible
	fakeStartReady
	fakeInBattle
)

type fakeGen2MenuDecoder struct{}

func (fakeGen2MenuDecoder) DecodeMenuCursor(r game.MemoryReader) game.MenuCursorState {
	return game.MenuCursorState{
		Current: int(r.Peek8(fakeMenuCurrent)),
		Max:     int(r.Peek8(fakeMenuMax)),
	}
}

func (fakeGen2MenuDecoder) DecodeTwoOption(r game.MemoryReader) (game.TwoOptionState, bool) {
	if r.Peek8(fakePromptOpen) == 0 {
		return game.TwoOptionState{}, false
	}
	return game.TwoOptionState{Current: int(r.Peek8(fakeMenuCurrent))}, true
}

func (fakeGen2MenuDecoder) DecodeStartMenu(r game.MemoryReader) game.StartMenuState {
	return game.StartMenuState{
		Visible:  r.Peek8(fakeStartVisible) != 0,
		Ready:    r.Peek8(fakeStartReady) != 0,
		InBattle: r.Peek8(fakeInBattle) != 0,
		Cursor: game.MenuCursorState{
			Current: int(r.Peek8(fakeMenuCurrent)),
			Max:     int(r.Peek8(fakeMenuMax)),
		},
	}
}

type fakeMenuMachine struct {
	mem [16]byte
}

func (m *fakeMenuMachine) Peek8(addr uint16) byte { return m.mem[addr] }

func (m *fakeMenuMachine) PeekInto(addr uint16, dst []byte) {
	copy(dst, m.mem[int(addr):])
}

func (*fakeMenuMachine) StepFrame() {}

func (*fakeMenuMachine) StepFrames(int) {}

func (m *fakeMenuMachine) Tap(btn emu.Button, _, _ int) {
	switch btn {
	case emu.Down:
		if m.mem[fakeMenuCurrent] < m.mem[fakeMenuMax] {
			m.mem[fakeMenuCurrent]++
		}
	case emu.Up:
		if m.mem[fakeMenuCurrent] > 0 {
			m.mem[fakeMenuCurrent]--
		}
	case emu.A:
		m.mem[fakeSelected] = m.mem[fakeMenuCurrent] + 1
		if m.mem[fakePromptOpen] != 0 {
			m.mem[fakePromptOpen] = 0
		}
	case emu.Start:
		if m.mem[fakeInBattle] == 0 {
			m.mem[fakeStartVisible] = 1
			m.mem[fakeStartReady] = 1
		}
	}
}

func TestGenericMenuSelectionUsesSemanticDecoder(t *testing.T) {
	m := &fakeMenuMachine{}
	m.mem[fakeMenuCurrent] = 3
	m.mem[fakeMenuMax] = 5

	if err := selectMenuItemWithDecoder(m, fakeGen2MenuDecoder{}, 1); err != nil {
		t.Fatalf("select fake Gen-II menu item: %v", err)
	}
	if got := m.mem[fakeMenuCurrent]; got != 1 {
		t.Fatalf("cursor = %d, want 1", got)
	}
	if got := m.mem[fakeSelected]; got != 2 {
		t.Fatalf("selected marker = %d, want 2", got)
	}
}

func TestGenericTwoOptionUsesSemanticDecoderAndVerifiesConsumed(t *testing.T) {
	m := &fakeMenuMachine{}
	m.mem[fakeMenuCurrent] = 0
	m.mem[fakeMenuMax] = 1
	m.mem[fakePromptOpen] = 1

	if err := selectTwoOptionWithDecoder(m, fakeGen2MenuDecoder{}, 1); err != nil {
		t.Fatalf("select fake Gen-II two-option: %v", err)
	}
	if m.mem[fakePromptOpen] != 0 {
		t.Fatal("prompt still open after selection")
	}
	if got := m.mem[fakeSelected]; got != 2 {
		t.Fatalf("selected marker = %d, want option 1 marker 2", got)
	}
}

func TestGenericStartMenuOpenUsesSemanticDecoder(t *testing.T) {
	m := &fakeMenuMachine{}
	m.mem[fakeMenuMax] = 8 // deliberately not a Gen-I menu shape

	if err := waitForStartMenuWithDecoder(m, fakeGen2MenuDecoder{}); err != nil {
		t.Fatalf("open fake Gen-II start menu: %v", err)
	}
	state := (fakeGen2MenuDecoder{}).DecodeStartMenu(m)
	if !state.Ready || !state.Visible {
		t.Fatalf("start menu state = %+v, want visible and ready", state)
	}
	if state.Cursor.Max != 8 {
		t.Fatalf("start menu max = %d, want fake Gen-II shape 8", state.Cursor.Max)
	}
}

func TestGenericStartMenuRefusesBattle(t *testing.T) {
	m := &fakeMenuMachine{}
	m.mem[fakeInBattle] = 1

	if err := waitForStartMenuWithDecoder(m, fakeGen2MenuDecoder{}); err == nil {
		t.Fatal("start menu opened during battle")
	}
	if m.mem[fakeStartVisible] != 0 || m.mem[fakeStartReady] != 0 {
		t.Fatal("battle refusal changed start-menu state")
	}
}
