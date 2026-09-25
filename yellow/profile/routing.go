package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/worldmodel"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

func (*Profile) MapProvider(romData []byte) worldmodel.MapHeaderProvider {
	return yellowrom.NewWorldProvider(romData)
}

func (*Profile) DecodeLiveTopology(r game.MemoryReader) (game.LiveTopologyState, error) {
	return engine.DecodeLiveTopology(r)
}
