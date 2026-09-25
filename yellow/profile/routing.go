package profile

import (
	"github.com/maestroi/pokepilot/game"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/worldmodel"
)

func (*Profile) MapProvider(romData []byte) worldmodel.MapHeaderProvider {
	return yellowrom.NewWorldProvider(romData)
}

func (*Profile) DecodeLiveTopology(r game.MemoryReader) (game.LiveTopologyState, error) {
	return engine.DecodeLiveTopology(r)
}
