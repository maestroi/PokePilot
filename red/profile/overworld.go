package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// DecodeOverworld keeps Red/Blue coordinate, movement-idle, battle and
// dialogue RAM details behind the profile boundary.
func (*Profile) DecodeOverworld(reader game.MemoryReader) game.OverworldState {
	if reader == nil {
		return game.OverworldState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	player := state.DecodePlayer(&mem)
	return game.OverworldState{
		NativeMapID:  uint16(player.MapID),
		X:            player.X,
		Y:            player.Y,
		Controllable: state.Controllable(&mem),
		MovementIdle: mem.U8(sym.WalkCounter) == 0 && mem.U8(sym.JoyIgnore) == 0 && mem.U8(sym.JoyHeld) == 0,
		InBattle:     state.DecodeBattle(&mem) != nil,
		InDialogue:   state.DecodeDialogue(&mem) != nil,
	}
}
