package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

var _ game.BattleExecutionDecoder = (*Profile)(nil)

func (p *Profile) DecodeBattleExecution(reader game.MemoryReader) game.BattleExecutionState {
	if reader == nil {
		return game.BattleExecutionState{}
	}

	inBattle := reader.Peek8(sym.BattleMode) != 0
	out := game.BattleExecutionState{InBattle: inBattle}
	count := int(reader.Peek8(sym.PartyCount))
	if count > 6 {
		count = 6
	}
	out.PartyMoves = make([][4]uint16, count)
	for i := 0; i < count; i++ {
		base := sym.PartyMon1 + uint16(i)*sym.PartyMonSize + gen2PartyMovesOffset
		for slot := 0; slot < 4; slot++ {
			out.PartyMoves[i][slot] = uint16(reader.Peek8(base + uint16(slot)))
		}
	}
	if !inBattle {
		return out
	}

	text := gsScreenText(reader)
	main := p.DecodeBattleMainMenu(reader)
	_, _, rows, cols, filter := gsMenuCursor(reader)
	moveMenu := reader.Peek8(sym.MoveSelectionMenuType) == 0 &&
		rows >= 1 && rows <= 4 && cols == 1 && filter == gen2BattleMoveFilter &&
		strings.Contains(text, "TYPE/")

	switch {
	case strings.Contains(text, "HM moves can't be"):
		out.Phase = game.BattleExecutionHMForgetRejected
	case strings.Contains(text, "Which move should") && strings.Contains(text, "be forgotten?"):
		out.Phase = game.BattleExecutionForgetMove
		cursor := p.DecodeMenuCursor(reader)
		// The forget list is four real move rows. Normalize it to 0..3 for
		// the portable replacement policy.
		if cursor.Current > 0 {
			cursor.Current--
		}
		cursor.Max = 3
		out.ForgetCursor = cursor
		out.ForgetReady = true
	case strings.Contains(text, "Can't escape!"):
		out.Phase = game.BattleExecutionRunRefused
	case strings.Contains(text, "is about to use") && strings.Contains(text, "change POK"):
		out.Phase = game.BattleExecutionTrainerSwitch
	case strings.Contains(text, "Stop learning"):
		out.Phase = game.BattleExecutionAbandonLearn
	case strings.Contains(text, "trying to learn"):
		out.Phase = game.BattleExecutionTryLearnPrompt
	case strings.Contains(text, "Use next POK"):
		out.Phase = game.BattleExecutionUseNextPrompt
	case strings.Contains(text, "The move is") && strings.Contains(text, "DISABLED!"):
		out.Phase = game.BattleExecutionMoveDisabled
	case moveMenu:
		out.Phase = game.BattleExecutionMoveMenu
	case main.Visible:
		out.Phase = game.BattleExecutionMainMenu
	}
	return out
}
