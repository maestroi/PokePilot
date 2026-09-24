package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const freshBootReadyMap uint8 = 0x26 // REDS_HOUSE_2F

// DecodeBootState keeps Red RAM addresses and its initial-ready map inside the
// Red profile while exposing only semantic menu/control state to the shared
// fresh-game driver.
func (*Profile) DecodeBootState(reader game.MemoryReader) game.BootState {
	if reader == nil {
		return game.BootState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	mapID := mem.U8(sym.CurMap)
	screen := state.ScreenText(&mem)
	presetNames := gen1.PresetMenuNames(screen)
	controllable := state.Controllable(&mem)
	return game.BootState{
		Ready:           mapID == freshBootReadyMap && controllable,
		Controllable:    controllable,
		NativeMapID:     uint16(mapID),
		MapName:         state.MapName(mapID),
		X:               mem.U8(sym.XCoord),
		Y:               mem.U8(sym.YCoord),
		MapWidth:        mem.U8(sym.CurMapWidth),
		MapHeight:       mem.U8(sym.CurMapHeight),
		FontLoaded:      mem.U8(sym.FontLoaded),
		NameMenu:        mem.U8(sym.MaxMenuItem) == 3 && strings.Contains(screen, "NEW NAME"),
		CurrentMenuItem: mem.U8(sym.CurrentMenuItem),
		MaxMenuItem:     mem.U8(sym.MaxMenuItem),
		PresetNames:     presetNames,
		PlayerName:      state.DecodeName(mem.Slice(sym.PlayerName, 11)),
		RivalName:       state.DecodeName(mem.Slice(sym.RivalName, 11)),
	}
}
