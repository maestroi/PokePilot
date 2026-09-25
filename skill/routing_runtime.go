package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/worldmodel"
)

func routingProfileFor(m *emu.Emu) (game.RoutingProfile, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: routing: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: routing: detect profile: %w", err)
	}
	routing, ok := profile.(game.RoutingProfile)
	if !ok {
		return nil, fmt.Errorf("skill: routing: profile %s@%s does not expose routing semantics", profile.ID(), profile.Revision())
	}
	return routing, nil
}

func routingProviderForROM(romData []byte) (worldmodel.MapHeaderProvider, error) {
	if profile, _, err := profiles.Detect(romData); err == nil {
		if routing, ok := profile.(game.RoutingProfile); ok {
			if provider := routing.MapProvider(romData); provider != nil {
				return provider, nil
			}
		}
	}
	if provider, ok := worldmodel.ProviderForROM(romData); ok && provider != nil {
		return provider, nil
	}
	return nil, fmt.Errorf("skill: routing: no map provider for ROM")
}

func routingHeaderFor(m *emu.Emu, mapID uint8) (worldmodel.MapHeader, error) {
	routing, err := routingProfileFor(m)
	if err != nil {
		return worldmodel.MapHeader{}, err
	}
	provider := routing.MapProvider(m.ROM())
	if provider == nil {
		return worldmodel.MapHeader{}, fmt.Errorf("skill: routing: profile returned nil map provider")
	}
	h, err := provider.ParseMap(mapID)
	if err != nil {
		return worldmodel.MapHeader{}, err
	}
	return h, nil
}

func routingHeaderForROM(romData []byte, mapID uint8) (worldmodel.MapHeader, error) {
	profile, _, err := profiles.Detect(romData)
	if err != nil {
		return worldmodel.MapHeader{}, fmt.Errorf("skill: routing: detect profile: %w", err)
	}
	routing, ok := profile.(game.RoutingProfile)
	if !ok {
		return worldmodel.MapHeader{}, fmt.Errorf("skill: routing: profile %s@%s does not expose routing semantics", profile.ID(), profile.Revision())
	}
	provider := routing.MapProvider(romData)
	if provider == nil {
		return worldmodel.MapHeader{}, fmt.Errorf("skill: routing: profile returned nil map provider")
	}
	return provider.ParseMap(mapID)
}
