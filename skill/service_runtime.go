package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func shopDecoderFor(m *emu.Emu) (game.ShopDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: shop: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: shop: detect profile: %w", err)
	}
	decoder, ok := profile.(game.ShopDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: shop: profile %s@%s does not expose shop semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

func centerDecoderFor(m *emu.Emu) (game.CenterDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: heal: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: heal: detect profile: %w", err)
	}
	decoder, ok := profile.(game.CenterDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: heal: profile %s@%s does not expose Pokemon Center semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

func shopItemPosition(state game.ShopState, item uint16) (int, bool) {
	for i, id := range state.Items {
		if id == item {
			return i, true
		}
	}
	return 0, false
}
