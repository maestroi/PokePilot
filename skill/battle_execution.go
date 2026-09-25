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
