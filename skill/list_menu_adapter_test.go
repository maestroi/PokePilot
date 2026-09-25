package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const (
	fakeListVisible uint16 = 72 + iota
	fakeListPosition
	fakeListSelected
)

type fakeGen2ListMenuDecoder struct{}

func (fakeGen2ListMenuDecoder) DecodeListMenu(r game.MemoryReader) game.ListMenuState {
	return game.ListMenuState{
		Visible:  r.Peek8(fakeListVisible) != 0,
		Kind:     game.ListMenuItems,
		Position: int(r.Peek8(fakeListPosition)),
	}
}

type fakeListMenuMachine struct {
	mem [96]byte
}

func (m *fakeListMenuMachine) Peek8(addr uint16) byte { return m.mem[addr] }

func (m *fakeListMenuMachine) PeekInto(addr uint16, dst []byte) {
	copy(dst, m.mem[int(addr):])
}

func (*fakeListMenuMachine) StepFrame() {}

func (*fakeListMenuMachine) StepFrames(int) {}

func (m *fakeListMenuMachine) Tap(btn emu.Button, _, _ int) {
	switch btn {
	case emu.Up:
		if m.mem[fakeListPosition] > 0 {
			m.mem[fakeListPosition]--
		}
	case emu.Down:
		m.mem[fakeListPosition]++
	case emu.A:
		m.mem[fakeListSelected] = m.mem[fakeListPosition] + 1
	}
}

func TestGenericScrollingListUsesSemanticAbsolutePosition(t *testing.T) {
	m := &fakeListMenuMachine{}
	m.mem[fakeListVisible] = 1
	m.mem[fakeListPosition] = 1

	if err := selectScrollingListEntryWithDecoder(m, fakeGen2ListMenuDecoder{}, 6); err != nil {
		t.Fatalf("select fake Gen-II list entry: %v", err)
	}
	if got := m.mem[fakeListPosition]; got != 6 {
		t.Fatalf("position = %d, want 6", got)
	}
	if got := m.mem[fakeListSelected]; got != 7 {
		t.Fatalf("selected marker = %d, want 7", got)
	}
}

func TestGenericScrollingListCanMoveUp(t *testing.T) {
	m := &fakeListMenuMachine{}
	m.mem[fakeListVisible] = 1
	m.mem[fakeListPosition] = 5

	if err := selectScrollingListEntryWithDecoder(m, fakeGen2ListMenuDecoder{}, 2); err != nil {
		t.Fatalf("select earlier entry: %v", err)
	}
	if got := m.mem[fakeListPosition]; got != 2 {
		t.Fatalf("position = %d, want 2", got)
	}
}

func TestGenericScrollingListRequiresVisibleMenu(t *testing.T) {
	m := &fakeListMenuMachine{}
	if err := selectScrollingListEntryWithDecoder(m, fakeGen2ListMenuDecoder{}, 0); err == nil {
		t.Fatal("hidden list menu was accepted")
	}
}

func TestGenericScrollingListRejectsNegativeIndexWithoutInput(t *testing.T) {
	m := &fakeListMenuMachine{}
	m.mem[fakeListVisible] = 1
	m.mem[fakeListPosition] = 3

	if err := selectScrollingListEntryWithDecoder(m, fakeGen2ListMenuDecoder{}, -1); err == nil {
		t.Fatal("negative list index was accepted")
	}
	if m.mem[fakeListPosition] != 3 || m.mem[fakeListSelected] != 0 {
		t.Fatal("negative selection changed list state")
	}
}
