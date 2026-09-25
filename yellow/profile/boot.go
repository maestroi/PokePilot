package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const freshBootReadyMap uint8 = 0x26 // REDS_HOUSE_2F

func readBytes(reader game.MemoryReader, addr uint16, n int) []byte {
	buf := make([]byte, n)
	reader.PeekInto(addr, buf)
	return buf
}

func yellowControllable(reader game.MemoryReader) bool {
	return reader.Peek8(sym.CurMapWidth) != 0 &&
		reader.Peek8(sym.CurMapHeight) != 0 &&
		reader.Peek8(sym.FontLoaded) == 0 &&
		reader.Peek8(sym.JoyIgnore) == 0 &&
		reader.Peek8(sym.WalkCounter) == 0
}

// DecodeBootState is Yellow's implementation of the shared semantic fresh-game
// contract. Only the profile knows these addresses and the initial bedroom map.
func (*Profile) DecodeBootState(reader game.MemoryReader) game.BootState {
	if reader == nil {
		return game.BootState{}
	}
	reader = native(reader)
	mapID := reader.Peek8(sym.CurMap)
	screen := gen1.NormalizeDisplayText(gen1.DecodeTiles(readBytes(reader, sym.TileMap, sym.TileMapLen)))
	presetNames := gen1.PresetMenuNames(screen)
	controllable := yellowControllable(reader)
	mapName, _ := (parser{}).MapName(uint16(mapID))
	return game.BootState{
		Ready:           mapID == freshBootReadyMap && controllable,
		Controllable:    controllable,
		NativeMapID:     uint16(mapID),
		MapName:         mapName,
		X:               reader.Peek8(sym.XCoord),
		Y:               reader.Peek8(sym.YCoord),
		MapWidth:        reader.Peek8(sym.CurMapWidth),
		MapHeight:       reader.Peek8(sym.CurMapHeight),
		FontLoaded:      reader.Peek8(sym.FontLoaded),
		NameMenu:        reader.Peek8(sym.MaxMenuItem) == 3 && strings.Contains(screen, "NEW NAME"),
		CurrentMenuItem: reader.Peek8(sym.CurrentMenuItem),
		MaxMenuItem:     reader.Peek8(sym.MaxMenuItem),
		PresetNames:     presetNames,
		PlayerName:      gen1.DecodeName(readBytes(reader, sym.PlayerName, 11)),
		RivalName:       gen1.DecodeName(readBytes(reader, sym.RivalName, 11)),
	}
}
