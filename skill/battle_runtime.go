package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func battleRuntimeDecoderFor(m *emu.Emu) (game.BattleRuntimeDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: battle runtime: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: battle runtime: detect profile: %w", err)
	}
	decoder, ok := profile.(game.BattleRuntimeDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: battle runtime: profile %s@%s does not expose battle runtime semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

func battleRuntimeContext(live game.BattleRuntimeState) string {
	return fmt.Sprintf("map %04x at (%d,%d)", live.NativeMapID, live.X, live.Y)
}
