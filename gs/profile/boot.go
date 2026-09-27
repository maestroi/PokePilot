package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gsNameTerminator      = 0x50
	gsFirstPresetMenuRow  = 2
	gsNameMenuRows        = 5
	gsContinueMenuMinRows = 3
)

func gsFreshBootMap() (gsdata.MapInfo, bool) {
	return gsdata.MapByName("PLAYERS_HOUSE_2F")
}

func gsPresetNames(id game.GameID) []string {
	switch id {
	case SilverGameID:
		return []string{"NEW NAME", "SILVER", "KAMON", "OSCAR", "MAX"}
	default:
		return []string{"NEW NAME", "GOLD", "HIRO", "TAYLOR", "KARL"}
	}
}

func decodeGSName(raw []byte) string {
	out := make([]byte, 0, len(raw))
	for _, b := range raw {
		if b == gsNameTerminator || b == 0 {
			break
		}
		switch {
		case b >= 0x80 && b <= 0x99:
			out = append(out, 'A'+(b-0x80))
		case b >= 0xa0 && b <= 0xb9:
			out = append(out, 'a'+(b-0xa0))
		case b == 0x7f:
			out = append(out, ' ')
		default:
			// Fresh-game preset names use only the alphabet above. Stop rather
			// than inventing text if a different naming surface owns the bytes.
			return string(out)
		}
	}
	return string(out)
}

func readGSBytes(reader game.MemoryReader, addr uint16, n int) []byte {
	buf := make([]byte, n)
	reader.PeekInto(addr, buf)
	return buf
}

// DecodeBootState projects Gold/Silver's distinct new-game flow into the
// shared boot contract. A is valid for the title screen, default 10:00 clock,
// confirmation prompts and Oak's dialogue. The only directional choices we
// own are NEW GAME when a save exists and the first built-in player name.
func (p *Profile) DecodeBootState(reader game.MemoryReader) game.BootState {
	if reader == nil {
		return game.BootState{}
	}

	mapID := gsdata.NativeMapID(reader.Peek8(sym.MapGroup), reader.Peek8(sym.MapNumber))
	mapInfo, _ := gsdata.Map(mapID)
	home, homeOK := gsFreshBootMap()
	homeID := uint16(0)
	if homeOK {
		homeID = gsdata.NativeMapID(home.Group, home.Number)
	}

	controllable := gsControllable(reader)
	cursor := reader.Peek8(sym.MenuCursorY)
	rows := reader.Peek8(sym.TwoDMenuNumRows)
	playerName := decodeGSName(readGSBytes(reader, sym.PlayerName, sym.PlayerNameLen))

	state := game.BootState{
		Ready:           homeOK && mapID == homeID && controllable,
		Controllable:    controllable,
		NativeMapID:     mapID,
		MapName:         mapInfo.Name,
		X:               reader.Peek8(sym.XCoord),
		Y:               reader.Peek8(sym.YCoord),
		MapWidth:        reader.Peek8(sym.MapWidth),
		MapHeight:       reader.Peek8(sym.MapHeight),
		CurrentMenuItem: cursor,
		MaxMenuItem:     rows,
		PlayerName:      playerName,
		NextInput:       game.BootInputConfirm,
	}

	if state.Ready {
		state.NextInput = game.BootInputWait
		return state
	}
	if homeOK && mapID == homeID {
		// Once the bedroom map is loading, no more intro input should leak into
		// the first controllable overworld frame.
		state.NextInput = game.BootInputWait
		return state
	}

	// NamePlayer's static menu has exactly five rows: NEW NAME plus four
	// version-specific presets. New-game WRAM clears wPlayerName before it.
	if playerName == "" && rows == gsNameMenuRows && cursor >= 1 && cursor <= gsNameMenuRows {
		names := gsPresetNames(p.id)
		state.NameMenu = true
		state.PresetNames = names
		switch {
		case cursor < gsFirstPresetMenuRow:
			state.NextInput = game.BootInputDown
		case cursor > gsFirstPresetMenuRow:
			state.NextInput = game.BootInputUp
		default:
			state.NextInput = game.BootInputConfirm
			state.SelectedPresetName = names[gsFirstPresetMenuRow-1]
		}
		return state
	}

	// With an existing battery save, Gold/Silver put CONTINUE above NEW GAME.
	// A fresh emulator normally has no save, but choosing row 2 makes the
	// fresh-game contract deterministic even when SRAM was supplied.
	if reader.Peek8(sym.SaveFileExists) != 0 && rows >= gsContinueMenuMinRows &&
		cursor >= 1 && cursor <= rows {
		switch {
		case cursor < 2:
			state.NextInput = game.BootInputDown
		case cursor > 2:
			state.NextInput = game.BootInputUp
		default:
			state.NextInput = game.BootInputConfirm
		}
	}

	return state
}

var _ game.BootProfile = (*Profile)(nil)
