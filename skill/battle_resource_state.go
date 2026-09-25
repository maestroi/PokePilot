package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func battleResourcesDecoderFor(m *emu.Emu) (game.BattleResourcesDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: battle resources: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: battle resources: detect profile: %w", err)
	}
	decoder, ok := profile.(game.BattleResourcesDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: battle resources: profile %s@%s does not expose battle resources", profile.ID(), profile.Revision())
	}
	return decoder, nil
}
