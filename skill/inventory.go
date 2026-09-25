package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func inventoryDecoderFor(m *emu.Emu) (game.InventoryDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: inventory: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: inventory: detect profile: %w", err)
	}
	decoder, ok := profile.(game.InventoryDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: inventory: profile %s@%s does not expose inventory semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}
