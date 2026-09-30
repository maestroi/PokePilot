package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

func (*Profile) MapProvider(romData []byte) worldmodel.MapHeaderProvider {
	return yellowrom.NewWorldProvider(romData)
}

// MapAdjacency exposes the static map adjacency in the portable uint16 width.
// Gen I map ids fit in the low byte; they are widened so the adjacency is
// uniform across games and generic routing never narrows a native id.
func (p *Profile) MapAdjacency(romData []byte) (map[uint16][]uint16, error) {
	graph, err := world.BuildGraph(p.MapProvider(romData))
	if err != nil {
		return nil, fmt.Errorf("yellow profile: build map graph: %w", err)
	}
	adjacency := make(map[uint16][]uint16, len(graph.Edges))
	for from, edges := range graph.Edges {
		for _, e := range edges {
			adjacency[uint16(from)] = append(adjacency[uint16(from)], uint16(e.To))
		}
	}
	return adjacency, nil
}

func (*Profile) DecodeLiveTopology(r game.MemoryReader) (game.LiveTopologyState, error) {
	return engine.DecodeLiveTopology(r)
}

func (*Profile) ElevatorTransitionReady(r game.MemoryReader, transition game.ElevatorTransition) bool {
	return engine.ElevatorTransitionReady(r, transition)
}
