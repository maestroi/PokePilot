package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func battleStateDecoderFor(m *emu.Emu) (game.BattleStateDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: battle state: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: battle state: detect profile: %w", err)
	}
	decoder, ok := profile.(game.BattleStateDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: battle state: profile %s@%s does not expose live battle state", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

func decodeBattleState(r game.MemoryReader, decoder game.BattleStateDecoder) (game.BattleState, bool) {
	if r == nil || decoder == nil {
		return game.BattleState{}, false
	}
	return decoder.DecodeBattleState(r)
}
