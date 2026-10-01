package profile

import (
	"github.com/maestroi/pokepilot/game"
	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

func (p *Profile) MapProvider(romData []byte) worldmodel.MapHeaderProvider {
	return redrom.NewWorldProvider(romData)
}

func (p *Profile) MapAdjacency(romData []byte) (map[uint16][]uint16, error) {
	graph, err := world.BuildGraph(p.MapProvider(romData))
	if err != nil {
		return nil, err
	}
	adjacency := make(map[uint16][]uint16, len(graph.Edges))
	for from, edges := range graph.Edges {
		for _, edge := range edges {
			adjacency[uint16(from)] = append(adjacency[uint16(from)], uint16(edge.To))
		}
	}
	return adjacency, nil
}

func (p *Profile) DecodeLiveTopology(r game.MemoryReader) (game.LiveTopologyState, error) {
	return p.engine.DecodeLiveTopology(r)
}

func (p *Profile) ElevatorTransitionReady(r game.MemoryReader, transition game.ElevatorTransition) bool {
	return p.engine.ElevatorTransitionReady(r, transition)
}
