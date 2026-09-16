package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

func init() {
	for _, id := range gen1Games {
		registerKnowledgeTopologyProvider(id, &redObjectiveAdapter{gameID: id})
	}
}

func redLocationID(id game.GameID, native uint8) LocationID {
	if name := state.MapName(native); name != "" {
		return LocationID(semanticLocation(name))
	}
	return LocationID(fmt.Sprintf("%s/map/%02x", id, native))
}

func (a *redObjectiveAdapter) KnowledgeTopology(native map[uint8][]uint8) KnowledgeTopology {
	topology := KnowledgeTopology{
		Adjacency:       map[LocationID][]LocationID{},
		NativeLocations: map[uint8]LocationID{},
	}
	location := func(id uint8) LocationID {
		if known := topology.NativeLocations[id]; known != "" {
			return known
		}
		semantic := redLocationID(a.gameID, id)
		topology.NativeLocations[id] = semantic
		return semantic
	}
	for from, neighbors := range native {
		fromID := location(from)
		for _, to := range neighbors {
			topology.Adjacency[fromID] = append(topology.Adjacency[fromID], location(to))
		}
	}
	return normalizeKnowledgeTopology(topology)
}
