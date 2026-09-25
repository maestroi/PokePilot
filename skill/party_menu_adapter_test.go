package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const (
	fakePartyVisible uint16 = 48 + iota
	fakePartyKind
	fakePartyCursor
	fakePartyMax
)

type fakeGen2PartyMenuDecoder struct{}

func (fakeGen2PartyMenuDecoder) DecodePartyMenu(r game.MemoryReader) game.PartyMenuState {
	if r.Peek8(fakePartyVisible) == 0 {
		return game.PartyMenuState{}
	}
	kind := game.PartyMenuForcedBattle
	switch r.Peek8(fakePartyKind) {
	case 1:
		kind = game.PartyMenuVoluntaryBattle
	case 2:
		kind = game.PartyMenuItemUse
	}
	return game.PartyMenuState{
		Visible: true,
		Kind:    kind,
		Cursor: game.MenuCursorState{
			Current: int(r.Peek8(fakePartyCursor)),
			Max:     int(r.Peek8(fakePartyMax)),
		},
	}
}

type fakePartyMenuMachine struct {
	mem [64]byte
}

func (m *fakePartyMenuMachine) Peek8(addr uint16) byte { return m.mem[addr] }

func (m *fakePartyMenuMachine) PeekInto(addr uint16, dst []byte) {
	copy(dst, m.mem[int(addr):])
}

func (*fakePartyMenuMachine) StepFrame() {}

func (*fakePartyMenuMachine) StepFrames(int) {}

func (m *fakePartyMenuMachine) Tap(btn emu.Button, _, _ int) {
	switch btn {
	case emu.Up:
		if m.mem[fakePartyCursor] > 0 {
			m.mem[fakePartyCursor]--
		}
	case emu.Down:
		if m.mem[fakePartyCursor] < m.mem[fakePartyMax] {
			m.mem[fakePartyCursor]++
		}
	case emu.A:
		m.mem[fakePartyVisible] = 0
	}
}

func TestGenericPartySlotSelectionUsesProfileCursor(t *testing.T) {
	for _, kind := range []byte{0, 1, 2} {
		m := &fakePartyMenuMachine{}
		m.mem[fakePartyVisible] = 1
		m.mem[fakePartyKind] = kind
		m.mem[fakePartyCursor] = 4
		m.mem[fakePartyMax] = 5

		if err := selectPartySlotWithDecoder(m, fakeGen2PartyMenuDecoder{}, 1); err != nil {
			t.Fatalf("kind %d: select party slot: %v", kind, err)
		}
		if m.mem[fakePartyCursor] != 1 {
			t.Fatalf("kind %d: cursor = %d, want 1", kind, m.mem[fakePartyCursor])
		}
		if m.mem[fakePartyVisible] != 0 {
			t.Fatalf("kind %d: party menu still visible after confirmation", kind)
		}
	}
}

func TestGenericPartySlotRejectsOutOfRangeWithoutInput(t *testing.T) {
	m := &fakePartyMenuMachine{}
	m.mem[fakePartyVisible] = 1
	m.mem[fakePartyCursor] = 2
	m.mem[fakePartyMax] = 4

	if err := selectPartySlotWithDecoder(m, fakeGen2PartyMenuDecoder{}, 5); err == nil {
		t.Fatal("out-of-range party slot was accepted")
	}
	if m.mem[fakePartyCursor] != 2 || m.mem[fakePartyVisible] != 1 {
		t.Fatal("out-of-range selection changed party-menu state")
	}
}

func TestGenericPartySlotRequiresVisibleMenu(t *testing.T) {
	m := &fakePartyMenuMachine{}
	if err := selectPartySlotWithDecoder(m, fakeGen2PartyMenuDecoder{}, 0); err == nil {
		t.Fatal("hidden party menu was accepted")
	}
}
