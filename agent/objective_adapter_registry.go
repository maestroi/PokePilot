package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
)

// ObjectiveAdapterFactory binds the portable objective runtime to one concrete
// game implementation for a live machine. The generic run loop selects this
// factory by GameID; game-specific construction stays in adapter registration.
type ObjectiveAdapterFactory func(*emu.Emu, []byte, RoutePriority) ObjectiveGameAdapter

var objectiveAdapterFactories = map[gameruntime.GameID]ObjectiveAdapterFactory{}

func registerObjectiveAdapterFactory(id gameruntime.GameID, factory ObjectiveAdapterFactory) {
	if id == "" || factory == nil {
		return
	}
	objectiveAdapterFactories[id] = factory
}

func objectiveAdapterFactoryFor(id gameruntime.GameID) (ObjectiveAdapterFactory, error) {
	factory, ok := objectiveAdapterFactories[id]
	if !ok {
		return nil, fmt.Errorf("agent: no objective adapter registered for game %q", id)
	}
	return factory, nil
}
