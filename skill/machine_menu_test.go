package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const (
	fakeMachinePocket uint16 = 8 + iota
	fakeMachineReady
	fakeMachinePosition
	fakeMachineCount
	fakeMachineOwned
)

type fakeMachineMenuDecoder struct{}

func (fakeMachineMenuDecoder) DecodeMachineMenu(r game.MemoryReader) game.MachineMenuState {
	raw := r.Peek8(fakeMachinePocket)
	pocket := game.MachinePocketUnknown
	switch raw {
	case 0:
		pocket = game.MachinePocketItems
	case 1:
		pocket = game.MachinePocketBalls
	case 2:
		pocket = game.MachinePocketKeyItems
	case 3:
		pocket = game.MachinePocketTMHM
	}
	return game.MachineMenuState{
		Visible:  true,
		Pocket:   pocket,
		Ready:    r.Peek8(fakeMachineReady) != 0,
		Position: int(r.Peek8(fakeMachinePosition)),
		Count:    int(r.Peek8(fakeMachineCount)),
	}
}

func (fakeMachineMenuDecoder) MachineMenuEntryIndex(r game.MemoryReader, native game.NativeFieldMove) (int, bool) {
	if native.MachineItemID != 0xf3 || r.Peek8(fakeMachineOwned) == 0 {
		return 0, false
	}
	return 2, true
}

type fakeMachineMenuMachine struct {
	fakeMenuMachine
}

func (m *fakeMachineMenuMachine) Tap(btn emu.Button, hold, gap int) {
	switch btn {
	case emu.Left:
		pocket := m.mem[fakeMachinePocket]
		if pocket == 0 {
			pocket = 3
		} else {
			pocket--
		}
		m.mem[fakeMachinePocket] = pocket
		m.mem[fakeMachineReady] = 0
		if pocket == 3 {
			m.mem[fakeMachineReady] = 1
		}
	case emu.Up:
		if m.mem[fakeMachineReady] != 0 && m.mem[fakeMachinePosition] > 0 {
			m.mem[fakeMachinePosition]--
		}
	case emu.Down:
		if m.mem[fakeMachineReady] != 0 &&
			m.mem[fakeMachinePosition]+1 < m.mem[fakeMachineCount] {
			m.mem[fakeMachinePosition]++
		}
	case emu.A:
		if m.mem[fakeMachineReady] != 0 {
			m.mem[fakeSelected] = m.mem[fakeMachinePosition] + 1
			m.mem[fakeMachineReady] = 0
		}
	default:
		m.fakeMenuMachine.Tap(btn, hold, gap)
	}
}

func TestSemanticMachineMenuNavigationReachesSparseOwnedEntry(t *testing.T) {
	m := &fakeMachineMenuMachine{}
	m.mem[fakeMachinePocket] = 2 // key items
	m.mem[fakeMachineCount] = 4
	m.mem[fakeMachinePosition] = 0
	m.mem[fakeMachineOwned] = 1

	err := selectMachineEntryWithDecoder(
		m,
		fakeMachineMenuDecoder{},
		game.NativeFieldMove{MachineItemID: 0xf3, MoveID: 0x0f},
	)
	if err != nil {
		t.Fatalf("select semantic machine entry: %v", err)
	}
	if got := m.mem[fakeMachinePocket]; got != 3 {
		t.Fatalf("pocket=%d, want TM/HM pocket 3 in fake", got)
	}
	if got := m.mem[fakeMachinePosition]; got != 2 {
		t.Fatalf("position=%d, want sparse owned index 2", got)
	}
	if got := m.mem[fakeSelected]; got != 3 {
		t.Fatalf("selected marker=%d, want third owned machine", got)
	}
	if m.mem[fakeMachineReady] != 0 {
		t.Fatal("machine action submenu did not open after selecting target")
	}
}

func TestSemanticMachineMenuRejectsUnownedMachine(t *testing.T) {
	m := &fakeMachineMenuMachine{}
	m.mem[fakeMachinePocket] = 3
	m.mem[fakeMachineReady] = 1
	m.mem[fakeMachineCount] = 1

	err := selectMachineEntryWithDecoder(
		m,
		fakeMachineMenuDecoder{},
		game.NativeFieldMove{MachineItemID: 0xf3, MoveID: 0x0f},
	)
	if err == nil {
		t.Fatal("unowned native machine was accepted")
	}
	if m.mem[fakeSelected] != 0 {
		t.Fatal("unowned machine changed selection state")
	}
}
