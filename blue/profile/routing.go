package profile

import (
	"github.com/maestroi/pokepilot/game"
	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/worldmodel"
)

func (p *Profile) MapProvider(romData []byte) worldmodel.MapHeaderProvider {
	return redrom.NewWorldProvider(romData)
}

func (p *Profile) DecodeLiveTopology(r game.MemoryReader) (game.LiveTopologyState, error) {
	return p.engine.DecodeLiveTopology(r)
}

func (p *Profile) ElevatorTransitionReady(r game.MemoryReader, transition game.ElevatorTransition) bool {
	return p.engine.ElevatorTransitionReady(r, transition)
}
