package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	// pret/pokegold constants/ram_constants.asm MAPSTATUS_*: the overworld map
	// shell is rebuilt during ENTER and only readable as geometry from HANDLE.
	gen2MapStatusStart  = 0
	gen2MapStatusEnter  = 1
	gen2MapStatusHandle = 2
	gen2MapStatusDone   = 3
	gen2MapEventsOn     = 0
	gen2ScriptOff       = 0

	gen2PlayerStepMidair   = 1 << 4
	gen2PlayerStepContinue = 1 << 5
	gen2PlayerStepStop     = 1 << 6
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
	// A script that takes the overworld as a step finishes (Elm's phone call)
	// freezes the flags at CONTINUE|STOP: the player is not moving however long
	// its text stays up. STOP alone is not idle, it is also set on the last
	// frames of an ordinary step, so this needs the script and its text box.
	if flags&gen2PlayerStepStop != 0 && flags&(gen2PlayerStepMidair|gen2PlayerStepStart) == 0 &&
		reader.Peek8(sym.MapStatus) == gen2MapStatusHandle && gsScriptActive(reader) && gsTextboxVisible(reader) {
		return true
	}
	return flags&(gen2PlayerStepMidair|gen2PlayerStepContinue|gen2PlayerStepStart) == 0
}

// gsScriptActive reports that a map script owns the machine, whether or not it
// is waiting on the player. It is deliberately broader than InDialogue: warps,
// stair transitions and cutscene force-walks run scripts while the ROM still
// owns movement.
func gsScriptActive(reader game.MemoryReader) bool {
	if reader == nil {
		return false
	}
	return reader.Peek8(sym.ScriptMode) != gen2ScriptOff ||
		reader.Peek8(sym.ScriptRunning) != 0 ||
		reader.Peek8(sym.ScriptFlags)&gen2ScriptRunningFlag != 0
}

// DecodeOverworld keeps Gen-II control-state and map-group semantics behind
// the Gold/Silver profile boundary.
func (*Profile) DecodeOverworld(reader game.MemoryReader) game.OverworldState {
	if reader == nil {
		return game.OverworldState{}
	}
	nativeMap := gsdata.NativeMapID(reader.Peek8(sym.MapGroup), reader.Peek8(sym.MapNumber))
	scriptActive := gsScriptActive(reader)
	movementIdle := gsMovementIdle(reader)
	return game.OverworldState{
		NativeMapID:  nativeMap,
		X:            reader.Peek8(sym.XCoord),
		Y:            reader.Peek8(sym.YCoord),
		Facing:       decodeFacing(reader.Peek8(sym.PlayerDirection)),
		Controllable: gsControllable(reader),
		MovementIdle: movementIdle,
		InBattle:     reader.Peek8(sym.BattleMode) != 0,
		// InDialogue means a script is waiting for the player on a text or
		// prompt surface, not merely running. A warp, stair transition or
		// scripted force-walk runs a script while it still owns movement;
		// reporting that as a dialogue made generic edge crossing abort with
		// ErrDialogueInterrupted in the frames between stepping onto a warp
		// and the map actually changing. After landing, the cartridge can
		// keep ScriptRunning with idle step flags and no textbox while the
		// map shell finishes — treating that as dialogue made Gen-II route
		// settlers mash A and walk straight back through the warp they just
		// crossed (Route 35 ↔ Goldenrod Gate). Only a script that is idle on
		// a visible text/prompt surface is a dialogue the caller must answer.
		InDialogue: scriptActive && movementIdle && gsTextboxVisible(reader),
	}
}

// gsTextboxVisible reports the standard bottom dialogue frame: its four corner
// tiles, which no overworld metatile row draws together.
func gsTextboxVisible(reader game.MemoryReader) bool {
	at := func(col, row uint16) byte { return reader.Peek8(sym.TileMap + row*20 + col) }
	return at(0, 12) == 0x79 && at(19, 12) == 0x7b && at(0, 17) == 0x7d && at(19, 17) == 0x7e
}
