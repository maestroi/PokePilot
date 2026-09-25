package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/worldmodel"
)

type routingProfile interface {
	game.GameProfile
	game.RoutingDecoder
	game.OverworldDecoder
	MapProvider([]byte) worldmodel.MapHeaderProvider
}

type routingRuntimeState struct {
	Map          uint8
	X, Y         uint8
	Controllable bool
	InBattle     bool
	InDialogue   bool
}

func routingRuntimeStateWithDecoder(reader game.MemoryReader, decoder game.OverworldDecoder) (routingRuntimeState, error) {
	if decoder == nil {
		return routingRuntimeState{}, fmt.Errorf("skill: routing: nil overworld decoder")
	}
	state := decoder.DecodeOverworld(reader)
	if state.NativeMapID > 0xff {
		return routingRuntimeState{}, fmt.Errorf("skill: routing: native map id %#04x exceeds current routing range", state.NativeMapID)
	}
	return routingRuntimeState{
		Map:          uint8(state.NativeMapID),
		X:            state.X,
		Y:            state.Y,
		Controllable: state.Controllable,
		InBattle:     state.InBattle,
		InDialogue:   state.InDialogue,
	}, nil
}

func currentRoutingRuntime(m *emu.Emu) (routingRuntimeState, error) {
	routing, err := routingProfileFor(m)
	if err != nil {
		return routingRuntimeState{}, err
	}
	return routingRuntimeStateWithDecoder(m, routing)
}

func routingPlayerXY(m *emu.Emu) (uint8, uint8) {
	state, err := currentRoutingRuntime(m)
	if err != nil {
		return 0, 0
	}
	return state.X, state.Y
}

func elevatorTransitionDecoderFor(m *emu.Emu) (game.ElevatorTransitionDecoder, error) {
	routing, err := routingProfileFor(m)
	if err != nil {
		return nil, err
	}
	decoder, ok := any(routing).(game.ElevatorTransitionDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: routing: profile %s@%s does not expose elevator transition semantics", routing.ID(), routing.Revision())
	}
	return decoder, nil
}

func routingProfileFor(m *emu.Emu) (routingProfile, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: routing: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: routing: detect profile: %w", err)
	}
	routing, ok := profile.(routingProfile)
	if !ok {
		return nil, fmt.Errorf("skill: routing: profile %s@%s does not expose routing semantics", profile.ID(), profile.Revision())
	}
	return routing, nil
}

func routingProviderForROM(romData []byte) (worldmodel.MapHeaderProvider, error) {
	if profile, _, err := profiles.Detect(romData); err == nil {
		if routing, ok := profile.(routingProfile); ok {
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
	routing, ok := profile.(routingProfile)
	if !ok {
		return worldmodel.MapHeader{}, fmt.Errorf("skill: routing: profile %s@%s does not expose routing semantics", profile.ID(), profile.Revision())
	}
	provider := routing.MapProvider(romData)
	if provider == nil {
		return worldmodel.MapHeader{}, fmt.Errorf("skill: routing: profile returned nil map provider")
	}
	return provider.ParseMap(mapID)
}
