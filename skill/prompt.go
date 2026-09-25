package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func promptDecoderFor(m *emu.Emu) (game.PromptDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: prompt: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: prompt: detect profile: %w", err)
	}
	decoder, ok := profile.(game.PromptDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: prompt: profile %s@%s does not expose prompt semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}
