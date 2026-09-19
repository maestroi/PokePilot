package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

type objectiveAdapterFactory func(*emu.Emu, []byte) ObjectiveGameAdapter

var objectiveAdapterFactories = map[game.GameID]objectiveAdapterFactory{}

func registerObjectiveAdapter(id game.GameID, factory objectiveAdapterFactory) {
	if id == "" || factory == nil {
		panic("agent: invalid objective adapter registration")
	}
	if _, exists := objectiveAdapterFactories[id]; exists {
		panic("agent: duplicate objective adapter for " + string(id))
	}
	objectiveAdapterFactories[id] = factory
}

func objectiveAdapterForGame(id game.GameID, m *emu.Emu, romData []byte) (ObjectiveGameAdapter, error) {
	factory := objectiveAdapterFactories[id]
	if factory == nil {
		return nil, fmt.Errorf("agent: no objective adapter registered for game %q", id)
	}
	return factory(m, romData), nil
}

func objectiveAdapterForROM(m *emu.Emu, romData []byte) (ObjectiveGameAdapter, error) {
	profile, _, err := profiles.Detect(romData)
	if err != nil {
		return nil, fmt.Errorf("agent: detect objective game profile: %w", err)
	}
	return objectiveAdapterForGame(profile.ID(), m, romData)
}
