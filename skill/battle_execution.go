package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func battleExecutionDecoderFor(m *emu.Emu) (game.BattleExecutionDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: battle execution: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: battle execution: detect profile: %w", err)
	}
	decoder, ok := profile.(game.BattleExecutionDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: battle execution: profile %s@%s does not expose battle-execution semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

func battleExecutionPhase(m game.MemoryReader, decoder game.BattleExecutionDecoder) game.BattleExecutionPhase {
	if decoder == nil || m == nil {
		return game.BattleExecutionNone
	}
	return decoder.DecodeBattleExecution(m).Phase
}

// Compatibility helpers keep existing battle-adjacent callers source-stable
// while moving their UI classification behind the active profile. Hot battle
// loops resolve the decoder once and call it directly.
func mainMenuUp(m *emu.Emu) bool {
	decoder, err := battleExecutionDecoderFor(m)
	return err == nil && battleExecutionPhase(m, decoder) == game.BattleExecutionMainMenu
}

func moveMenuUp(m *emu.Emu) bool {
	decoder, err := battleExecutionDecoderFor(m)
	if err != nil {
		return false
	}
	phase := battleExecutionPhase(m, decoder)
	return phase == game.BattleExecutionMoveMenu || phase == game.BattleExecutionMoveDisabled
}

func disabledMoveRefusalUp(m *emu.Emu) bool {
	decoder, err := battleExecutionDecoderFor(m)
	return err == nil && battleExecutionPhase(m, decoder) == game.BattleExecutionMoveDisabled
}

func twoOptionPromptUp(m *emu.Emu) bool {
	decoder, err := battleExecutionDecoderFor(m)
	if err != nil {
		return false
	}
	switch battleExecutionPhase(m, decoder) {
	case game.BattleExecutionUseNextPrompt, game.BattleExecutionTryLearnPrompt:
		return true
	default:
		return false
	}
}

func abandonLearnPromptUp(m *emu.Emu) bool {
	decoder, err := battleExecutionDecoderFor(m)
	return err == nil && battleExecutionPhase(m, decoder) == game.BattleExecutionAbandonLearn
}

func trainerSwitchPromptUp(m *emu.Emu) bool {
	decoder, err := battleExecutionDecoderFor(m)
	return err == nil && battleExecutionPhase(m, decoder) == game.BattleExecutionTrainerSwitch
}

func forgetMenuUp(m *emu.Emu) bool {
	decoder, err := battleExecutionDecoderFor(m)
	return err == nil && battleExecutionPhase(m, decoder) == game.BattleExecutionForgetMove
}

func switchBoxUp(m *emu.Emu) bool {
	decoder, err := battleExecutionDecoderFor(m)
	return err == nil && battleExecutionPhase(m, decoder) == game.BattleExecutionSwitchBox
}


func selectFightEntry(m *emu.Emu) error {
	return selectBattleMainMenuEntry(m, game.BattleMenuFight)
}

// selectForgetSlot navigates the live replacement list through generic menu
// semantics. The profile owns both recognition of the forget surface and the
// cursor encoding.
func selectForgetSlot(m *emu.Emu, index int) error {
	execution, err := battleExecutionDecoderFor(m)
	if err != nil {
		return err
	}
	live := execution.DecodeBattleExecution(m)
	if live.Phase != game.BattleExecutionForgetMove {
		return fmt.Errorf("skill: selectForgetSlot: move-forget menu is not visible")
	}
	if !live.ForgetReady {
		if !waitMenuUntil(m, menuSettleFrames, func() bool {
			next := execution.DecodeBattleExecution(m)
			return next.Phase == game.BattleExecutionForgetMove && next.ForgetReady
		}) {
			return fmt.Errorf("skill: selectForgetSlot: move-forget menu did not become ready")
		}
		live = execution.DecodeBattleExecution(m)
	}
	if index < 0 || index > live.ForgetCursor.Max {
		return fmt.Errorf("skill: selectForgetSlot: slot %d out of range 0..%d", index, live.ForgetCursor.Max)
	}
	return SelectMenuItem(m, index)
}

// battleSwitchMenuUp is retained for battle-adjacent recovery callers, but
// its classification now comes from the profile-owned party menu capability.
func battleSwitchMenuUp(m *emu.Emu) bool {
	decoder, err := partyMenuDecoderFor(m)
	if err != nil {
		return false
	}
	live := decoder.DecodePartyMenu(m)
	return live.Visible && live.Kind == game.PartyMenuVoluntaryBattle
}
