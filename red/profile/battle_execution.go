package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	battleMainMenuMarker      = "FIGHT"
	battleMoveMenuMarker      = "TYPE/"
	battleDisabledMoveMarker  = "move is disabled"
	battleUseNextMarker       = "Use next"
	battleTryLearnMarker      = "trying to learn"
	battleAbandonLearnMarker  = "Abandon learning"
	battleTrainerSwitchMarker = "change POK"
	battleForgetMenuMarker    = "forgotten?"
	battleHMCantDeleteMarker  = "HM techniques"
	battleSwitchBoxMarker     = "SWITCH"
)

// DecodeBattleExecution keeps Red/Blue's battle tile markers, move-learning
// scratch variables and party move layout behind the profile boundary.
func (*Profile) DecodeBattleExecution(reader game.MemoryReader) game.BattleExecutionState {
	if reader == nil {
		return game.BattleExecutionState{}
	}

	var mem state.Mem
	reader.PeekInto(0, mem[:])
	text := state.ScreenText(&mem)
	menu := state.DecodeMenu(&mem)

	out := game.BattleExecutionState{
		InBattle:    state.DecodeBattle(&mem) != nil,
		OfferedMove: uint16(mem.U8(sym.MoveNum)),
	}
	party := state.DecodeParty(&mem)
	out.PartyMoves = make([][4]uint16, len(party.Mons))
	for i, mon := range party.Mons {
		for slot, move := range mon.Moves {
			out.PartyMoves[i][slot] = uint16(move)
		}
	}

	learnerSlot := int(mem.U8(sym.WhichPokemon))
	if learnerSlot >= 0 && learnerSlot < len(party.Mons) {
		mon := party.Mons[learnerSlot]
		out.Learner = game.BattleMoveLearnerState{
			Valid:     true,
			PartySlot: learnerSlot,
			Type1:     uint16(mon.Type1),
			Type2:     uint16(mon.Type2),
		}
		for slot, move := range mon.Moves {
			out.Learner.Moves[slot] = uint16(move)
		}
	}

	switch {
	case strings.Contains(text, battleHMCantDeleteMarker):
		out.Phase = game.BattleExecutionHMForgetRejected
	case strings.Contains(text, battleForgetMenuMarker):
		out.Phase = game.BattleExecutionForgetMove
		out.ForgetCursor = game.MenuCursorState{Current: menu.Current, Max: menu.Max}
		out.ForgetReady = menu.Max == 3
	case strings.Contains(text, battleSwitchBoxMarker):
		out.Phase = game.BattleExecutionSwitchBox
	case strings.Contains(text, battleTrainerSwitchMarker):
		out.Phase = game.BattleExecutionTrainerSwitch
	case strings.Contains(text, battleAbandonLearnMarker):
		out.Phase = game.BattleExecutionAbandonLearn
	case strings.Contains(text, battleTryLearnMarker):
		out.Phase = game.BattleExecutionTryLearnPrompt
	case strings.Contains(text, battleUseNextMarker):
		out.Phase = game.BattleExecutionUseNextPrompt
	case strings.Contains(text, battleDisabledMoveMarker):
		out.Phase = game.BattleExecutionMoveDisabled
	case strings.Contains(text, battleMoveMenuMarker):
		out.Phase = game.BattleExecutionMoveMenu
	case strings.Contains(text, battleMainMenuMarker):
		out.Phase = game.BattleExecutionMainMenu
	}
	return out
}
