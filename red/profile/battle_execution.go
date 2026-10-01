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
	battleRunRefusedMarker    = "running from a"
	battleForgetMenuMarker    = "forgotten?"
	battleHMCantDeleteMarker  = "HM techniques"
	battleSwitchBoxMarker     = "SWITCH"

	// PrintMenuItem replaces the TYPE/ panel with the Disabled marker at
	// hlcoord 1,10 when the move cursor rests on the Disabled move. The
	// position separates it from the "<move> was disabled!" battle message in
	// the text box. Red draws "disabled!"; Yellow draws "Disabled!" — match
	// case-insensitively so the shared Gen-I decoder accepts both cartridges
	// (run-2wka7km6oqsz62cajfp6yl6cuo stalled on select FIGHT while the Yellow
	// move menu was already open with the cursor on Slam).
	battleMovePanelDisabledMarker = "disabled!"
	battleMovePanelDisabledOffset = 10*20 + 1
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
		InBattle:             state.DecodeBattle(&mem) != nil,
		OfferedMove:          uint16(mem.U8(sym.MoveNum)),
		MoveSelectionSkipped: moveSelectionSkipped(&mem),
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
	case strings.Contains(text, battleRunRefusedMarker):
		out.Phase = game.BattleExecutionRunRefused
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
	case strings.Contains(text, battleMoveMenuMarker),
		movePanelShowsDisabled(&mem):
		out.Phase = game.BattleExecutionMoveMenu
	// FIGHT tiles alone are not proof that the player owns the command menu.
	// Gen-I scripted battles (notably Viridian's Old Man catch demo) draw the
	// same command-menu text while the ROM is supplying simulated input, but
	// leave wMaxMenuItem at the prior list shape (measured as 4). Reporting that
	// screen as player-actionable makes Battle() fight the ROM for the cursor and
	// can return with the script still owning the objective boundary. The real
	// FIGHT/ITEM/PKMN/RUN menu is exactly one row of two columns (max item 1),
	// the same positive shape DecodeBattleEscapeMenu already requires.
	case strings.Contains(text, battleMainMenuMarker) && menu.Max == int(redBattleMenuCommandMax):
		out.Phase = game.BattleExecutionMainMenu
	}
	return out
}

const (
	battleStatusFrozen       = 1 << 5 // FRZ in wBattleMonStatus
	battleStatusSleepMask    = 0b111  // SLP_MASK in wBattleMonStatus
	battleStatusBide         = 1 << 0 // STORING_ENERGY in wPlayerBattleStatus1
	battleStatusTrappingMove = 1 << 5 // USING_TRAPPING_MOVE in w*BattleStatus1
)

// movePanelShowsDisabled reports PrintMenuItem's Disabled panel at hlcoord
// 1,10. Red's DisabledText is "disabled!"; Yellow's is "Disabled!".
func movePanelShowsDisabled(mem *state.Mem) bool {
	if mem == nil {
		return false
	}
	panel := state.DecodeTiles(mem.Slice(sym.TileMap+battleMovePanelDisabledOffset, len(battleMovePanelDisabledMarker)))
	return strings.EqualFold(panel, battleMovePanelDisabledMarker)
}

// moveSelectionSkipped mirrors the checks MainInBattleLoop makes right after
// DisplayBattleMenu returns (engine/battle/core.asm): a sleeping or frozen
// active mon, the player's own Bide or Wrap, or an enemy Wrap all jump to
// .selectEnemyMove, so FIGHT runs the turn without drawing the move menu.
//
// MoveSelectionMenu.regularmenu calls AnyMoveToSelect first and returns before
// drawing anything when it selects STRUGGLE, so an active mon with no
// selectable move left skips the move menu the same way: FIGHT alone runs the
// turn. Missing that case stranded the Silph rival fight — Oddish was out of PP
// on every move, the ROM printed "has no moves left!" and started the Struggle
// turn, and the shared FIGHT wait read the absent move menu as a stuck menu
// (run-39etso0zuq4451wr2duvk128ph).
func moveSelectionSkipped(mem *state.Mem) bool {
	return mem.U8(sym.BattleMonStatus)&(battleStatusFrozen|battleStatusSleepMask) != 0 ||
		mem.U8(sym.PlayerBattleStatus1)&(battleStatusBide|battleStatusTrappingMove) != 0 ||
		mem.U8(sym.EnemyBattleStatus1)&battleStatusTrappingMove != 0 ||
		!anyMoveToSelect(mem)
}

// anyMoveToSelect mirrors engine/battle/core.asm AnyMoveToSelect: it is false
// exactly when the ROM picks STRUGGLE instead of drawing the move menu. A
// disabled move's PP is ignored because the cursor can never select it.
//
// The ROM ORs raw PP bytes when a move is disabled and masks them otherwise;
// masking both ways says "no move to select" for a spent move that carries PP
// Up bits. That is the safe direction: it hands the turn to the ROM's own
// STRUGGLE path instead of waiting for a move menu whose moves all refuse.
func anyMoveToSelect(mem *state.Mem) bool {
	disabled := int(mem.U8(sym.PlayerDisabledMove) >> 4)
	var pp uint8
	for slot := 0; slot < 4; slot++ {
		if slot+1 == disabled {
			continue
		}
		pp |= mem.U8(sym.BattleMonPP+uint16(slot)) & state.CurrentPPMask
	}
	return pp != 0
}
