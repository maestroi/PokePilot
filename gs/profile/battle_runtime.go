package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

var _ game.BattleRuntimeDecoder = (*Profile)(nil)

func (p *Profile) DecodeBattleRuntime(reader game.MemoryReader) game.BattleRuntimeState {
	if reader == nil {
		return game.BattleRuntimeState{}
	}
	inBattle := reader.Peek8(sym.BattleMode) != 0
	scriptActive := reader.Peek8(sym.ScriptMode) != gen2ScriptOff ||
		reader.Peek8(sym.ScriptRunning) != 0 ||
		reader.Peek8(sym.ScriptFlags)&gen2ScriptRunningFlag != 0
	nativeMap := gsdata.NativeMapID(reader.Peek8(sym.MapGroup), reader.Peek8(sym.MapNumber))
	text := gsScreenText(reader)
	return game.BattleRuntimeState{
		InBattle:      inBattle,
		Controllable:  gsControllable(reader),
		TextActive:    !inBattle && scriptActive,
		NativeMapID:   nativeMap,
		X:             reader.Peek8(sym.XCoord),
		Y:             reader.Peek8(sym.YCoord),
		DebugText:     fmt.Sprintf("%s", text),
		MenuCursor:    p.DecodeMenuCursor(reader),
	}
}
