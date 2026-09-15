package agent

import (
	"fmt"

	redprofile "github.com/maestroi/pokepilot/red/profile"
	"github.com/maestroi/pokepilot/red/state"
)

func init() {
	registerKnowledgeTopologyProvider(redprofile.GameID, &redObjectiveAdapter{})
}

func redLocationID(id uint8) LocationID {
	if name := state.MapName(id); name != "" {
		return LocationID(semanticLocation(name))
	}
	return LocationID(fmt.Sprintf("pokemon-red/map/%02x", id))
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
		semantic := redLocationID(id)
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
