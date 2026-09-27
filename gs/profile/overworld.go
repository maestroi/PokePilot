package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gen2MapStatusHandle = 2
	gen2MapEventsOn     = 0
	gen2ScriptOff       = 0

	gen2PlayerStepMidair   = 1 << 4
	gen2PlayerStepContinue = 1 << 5
	gen2PlayerStepStart    = 1 << 7
	gen2ScriptRunningFlag  = 1 << 2
)

func gsControllable(reader game.MemoryReader) bool {
	if reader == nil {
		return false
	}
	if reader.Peek8(sym.MapWidth) == 0 || reader.Peek8(sym.MapHeight) == 0 {
		return false
	}
	if reader.Peek8(sym.BattleMode) != 0 ||
		reader.Peek8(sym.MapStatus) != gen2MapStatusHandle ||
		reader.Peek8(sym.MapEventStatus) != gen2MapEventsOn ||
		reader.Peek8(sym.ScriptMode) != gen2ScriptOff ||
		reader.Peek8(sym.ScriptRunning) != 0 ||
		reader.Peek8(sym.ScriptFlags)&gen2ScriptRunningFlag != 0 {
		return false
	}
	return true
}

func gsMovementIdle(reader game.MemoryReader) bool {
	if reader == nil {
		return false
	}
	flags := reader.Peek8(sym.PlayerStepFlags)
	return flags&(gen2PlayerStepMidair|gen2PlayerStepContinue|gen2PlayerStepStart) == 0
}

// DecodeOverworld keeps Gen-II control-state and map-group semantics behind
// the Gold/Silver profile boundary.
func (*Profile) DecodeOverworld(reader game.MemoryReader) game.OverworldState {
	if reader == nil {
		return game.OverworldState{}
	}
	nativeMap := gsdata.NativeMapID(reader.Peek8(sym.MapGroup), reader.Peek8(sym.MapNumber))
	scriptActive := reader.Peek8(sym.ScriptMode) != gen2ScriptOff ||
		reader.Peek8(sym.ScriptRunning) != 0 ||
		reader.Peek8(sym.ScriptFlags)&gen2ScriptRunningFlag != 0
	return game.OverworldState{
		NativeMapID:  nativeMap,
		X:            reader.Peek8(sym.XCoord),
		Y:            reader.Peek8(sym.YCoord),
		Facing:       decodeFacing(reader.Peek8(sym.PlayerDirection)),
		Controllable: gsControllable(reader),
		MovementIdle: gsMovementIdle(reader),
		InBattle:     reader.Peek8(sym.BattleMode) != 0,
		InDialogue:   scriptActive,
	}
}
