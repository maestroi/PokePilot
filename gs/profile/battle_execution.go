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
	out := game.BattleExecutionState{
		InBattle:             inBattle,
		OfferedMove:          uint16(reader.Peek8(sym.PutativeTMHMMove)),
		MoveSelectionSkipped: inBattle && !gsPlayerHasUsableMoves(reader),
	}
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

	// Experience / LearnMove address a party slot via wCurPartyMon, which is
	// not always the active battler. Project that learner so shared Battle
	// execution can answer try-learn / forget prompts without Red WRAM.
	learnerSlot := int(reader.Peek8(sym.CurPartyMon))
	if learnerSlot >= 0 && learnerSlot < count {
		out.Learner = game.BattleMoveLearnerState{
			Valid:     true,
			PartySlot: learnerSlot,
			Moves:     out.PartyMoves[learnerSlot],
		}
		// Party structs omit types. Prefer the live battle struct when the
		// learner is the active mon; otherwise leave types unknown rather
		// than inventing BaseData reconstruction in the execution decoder.
		if inBattle && learnerSlot == int(reader.Peek8(sym.CurBattleMon)) {
			out.Learner.Type1 = uint16(reader.Peek8(sym.BattleMonType1))
			out.Learner.Type2 = uint16(reader.Peek8(sym.BattleMonType2))
		}
	}

	text := gsScreenText(reader)

	// LearnMove is shared by battle level-ups and out-of-battle TM/HM teaching.
	// Classify those surfaces before the battle-only early return so a native
	// machine executor can reuse the same semantic prompts and forget menu.
	switch {
	case strings.Contains(text, "HM moves can't be"):
		out.Phase = game.BattleExecutionHMForgetRejected
		return out
	case strings.Contains(text, "Which move should") && strings.Contains(text, "be forgotten?"):
		out.Phase = game.BattleExecutionForgetMove
		cursor := p.DecodeMenuCursor(reader)
		// Battle move-list decoding deliberately preserves Gen-II's native
		// 1-based cursor, while ordinary/out-of-battle vertical menus are
		// already normalized to zero-based. Only the battle form needs the
		// extra decrement here.
		if inBattle && cursor.Current > 0 {
			cursor.Current--
		}
		cursor.Max = 3
		out.ForgetCursor = cursor
		out.ForgetReady = true
		return out
	case strings.Contains(text, "Stop learning"):
		out.Phase = game.BattleExecutionAbandonLearn
		return out
	case strings.Contains(text, "trying to learn"):
		out.Phase = game.BattleExecutionTryLearnPrompt
		return out
	}

	if !inBattle {
		return out
	}

	main := p.DecodeBattleMainMenu(reader)
	_, _, rows, cols, filter := gsMenuCursor(reader)
	// MoveInfoBox draws "TYPE/" for a selectable move and "Disabled!" at
	// hlcoord 1,10 when the cursor rests on the Disabled slot. The panel
	// position separates that from "<move> was DISABLED!" in the text box;
	// matching the word anywhere made the announcement look like a move menu
	// and stuck SelectMenuItem (run-1or7lhbp9yldp, triage:77cfbd2342cb276e).
	moveInfo := strings.Contains(text, "TYPE/") || gsMovePanelShowsDisabled(reader)
	moveMenu := reader.Peek8(sym.MoveSelectionMenuType) == 0 &&
		rows >= 1 && rows <= 4 && cols == 1 && filter == gen2BattleMoveFilter &&
		moveInfo

	switch {
	case strings.Contains(text, "Can't escape!"):
		out.Phase = game.BattleExecutionRunRefused
	// The switch-confirmation prompt is recognized by its own marker, the
	// same way the Gen-I adapter does. "<TRAINER> is about to use <MON>." is
	// long enough to scroll off the 4-line battle box before the YES/NO
	// cursor is drawn, so by the time the prompt is answerable only "Will
	// <PLAYER> change POKéMON?" is on screen; requiring the scrolled-off
	// half left the prompt unowned and the blind A-tap confirmed YES
	// (run-11dd5ya1qev0ry, triage:e1ef45bf25e3b946).
	case strings.Contains(text, "change POK"):
		out.Phase = game.BattleExecutionTrainerSwitch
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

// gsPlayerHasUsableMoves mirrors MoveSelectionScreen's usable-move check in
// pret/pokegold engine/battle/core.asm: when every non-Disabled move's
// current-PP nibble is zero, FIGHT selects STRUGGLE and never draws the move
// menu. Shared Battle must treat that as MoveSelectionSkipped or it waits for
// a menu that will not open (run-1p7ixxdreiam630odlwlg1xf35,
// triage:20d487bea54de661). PP Up bits must not count as remaining PP.
//
// The check also covers multi-turn move locks: when the active mon is in a
// "charging" move (Bide, Fly, Dive, Solar Beam, …) or a recharge turn
// (Hyper Beam, Thrash, …), the ROM auto-executes the next phase and skips
// the move-selection menu entirely, regardless of PP. Without this, a mon
// that randomly selects Bide via Metronome leaves the agent waiting for a
// menu that will never open (run-b8p83zrdrdya3b1px3mcln8nm,
// triage:0d1ce8bc52e0f7a6).
func gsPlayerHasUsableMoves(reader game.MemoryReader) bool {
	if reader == nil {
		return false
	}
	// A mon locked in a multi-turn move skips the move-selection menu.
	if reader.Peek8(sym.PlayerCharging) != 0 {
		return false
	}
	if reader.Peek8(sym.PlayerTrappingMove) != 0 {
		return false
	}
	disabled := gsDisabledMoveSlot(reader)
	var pp byte
	for slot := 0; slot < 4; slot++ {
		if slot == disabled {
			continue
		}
		pp |= reader.Peek8(sym.BattleMonPP + uint16(slot))
	}
	return pp&gen2PPMask != 0
}

const (
	// MoveInfoBox's Disabled string is placed at hlcoord 1,10
	// (engine/battle/core.asm), the same panel corner Gen I uses.
	gsMovePanelDisabledOffset = 10*20 + 1
	gsMovePanelDisabledMarker = "Disabled!"
)

// gsMovePanelShowsDisabled reports MoveInfoBox's Disabled panel. Matching the
// word case-insensitively at that coordinate accepts the retail string without
// treating "<move> was DISABLED!" in the battle text box as the move menu.
func gsMovePanelShowsDisabled(reader game.MemoryReader) bool {
	if reader == nil {
		return false
	}
	raw := make([]byte, len(gsMovePanelDisabledMarker))
	reader.PeekInto(sym.TileMap+gsMovePanelDisabledOffset, raw)
	return strings.EqualFold(gsDecodeTiles(raw), gsMovePanelDisabledMarker)
}
