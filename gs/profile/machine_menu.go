package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gen2PocketItems    byte = 0
	gen2PocketBalls    byte = 1
	gen2PocketKeyItems byte = 2
	gen2PocketTMHM     byte = 3

	gen2PackStateTMHMPocketMenu byte = 8
	gen2TMHMMenuFilter               = gen2PadA | gen2PadB | 0xf0
)

// gsPackVisible uses the fixed 5x3 pack graphic written by PlacePackGFX as
// positive liveness proof. The pocket/jumptable bytes share unions with other
// screens and remain stale after menus close.
func gsPackVisible(reader game.MemoryReader) bool {
	if reader == nil {
		return false
	}
	want := byte(0x50)
	for row := uint16(3); row < 6; row++ {
		for col := uint16(0); col < 5; col++ {
			if reader.Peek8(sym.TileMap+row*20+col) != want {
				return false
			}
			want++
		}
	}
	return true
}

func gsMachinePocket(raw byte) game.MachinePocket {
	switch raw {
	case gen2PocketItems:
		return game.MachinePocketItems
	case gen2PocketBalls:
		return game.MachinePocketBalls
	case gen2PocketKeyItems:
		return game.MachinePocketKeyItems
	case gen2PocketTMHM:
		return game.MachinePocketTMHM
	default:
		return game.MachinePocketUnknown
	}
}

func gsOwnedMachineCount(reader game.MemoryReader) int {
	count := 0
	for i := 0; i < sym.TMsHMsCount; i++ {
		if reader.Peek8(sym.TMsHMs+uint16(i)) != 0 {
			count++
		}
	}
	return count
}

// DecodeMachineMenu projects Gold/Silver's PACK pocket and the custom TM/HM
// pocket cursor to a generation-neutral absolute owned-machine position.
func (*Profile) DecodeMachineMenu(reader game.MemoryReader) game.MachineMenuState {
	if reader == nil || !gsPackVisible(reader) {
		return game.MachineMenuState{Position: -1}
	}
	state := game.MachineMenuState{
		Visible:  true,
		Pocket:   gsMachinePocket(reader.Peek8(sym.CurPocket)),
		Position: -1,
		Count:    gsOwnedMachineCount(reader),
	}
	if state.Pocket != game.MachinePocketTMHM {
		return state
	}
	y, x, rows, cols, filter := gsMenuCursor(reader)
	state.Ready = reader.Peek8(sym.JumptableIndex) == gen2PackStateTMHMPocketMenu &&
		cols == 1 && rows >= 1 && rows <= 5 &&
		x == 1 && y >= 1 && y <= rows &&
		filter == gen2TMHMMenuFilter
	if !state.Ready {
		return state
	}
	state.Position = int(reader.Peek8(sym.TMHMPocketScroll)) + int(reader.Peek8(sym.TMHMPocketCursor))
	return state
}

// MachineMenuEntryIndex converts a native TM/HM item id into its absolute
// zero-based position among currently owned machines. Gen II's TM/HM pocket
// omits unowned entries, so native machine number is not itself a menu index.
func (*Profile) MachineMenuEntryIndex(reader game.MemoryReader, native game.NativeFieldMove) (int, bool) {
	if reader == nil || native.MachineItemID == 0 || native.MachineItemID > 0xff {
		return 0, false
	}
	machine := -1
	for i, item := range gsdata.MachineItems {
		if item == uint8(native.MachineItemID) {
			machine = i
			break
		}
	}
	if machine < 0 || machine >= sym.TMsHMsCount ||
		reader.Peek8(sym.TMsHMs+uint16(machine)) == 0 {
		return 0, false
	}
	index := 0
	for i := 0; i < machine; i++ {
		if reader.Peek8(sym.TMsHMs+uint16(i)) != 0 {
			index++
		}
	}
	return index, true
}

var _ game.MachineMenuDecoder = (*Profile)(nil)
