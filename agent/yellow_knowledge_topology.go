package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

type yellowKnowledgeTopologyProvider struct{}

func init() {
	registerKnowledgeTopologyProvider(yellowprofile.GameID, yellowKnowledgeTopologyProvider{})
}

func yellowLocationID(id game.GameID, native uint8) LocationID {
	if name := yellowrom.MapName(native); name != "" {
		return LocationID(semanticLocation(name))
	}
	return LocationID(fmt.Sprintf("%s/map/%02x", id, native))
}

func (yellowKnowledgeTopologyProvider) KnowledgeTopology(native map[uint8][]uint8) KnowledgeTopology {
	topology := KnowledgeTopology{
		Adjacency:       map[LocationID][]LocationID{},
		NativeLocations: map[uint8]LocationID{},
	}
	location := func(id uint8) LocationID {
		if known := topology.NativeLocations[id]; known != "" {
			return known
		}
		semantic := yellowLocationID(yellowprofile.GameID, id)
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

func yellowNativeMapForLocation(location LocationID) (uint8, bool) {
	if location == "" {
		return 0, false
	}
	for _, mapID := range yellowrom.MapIDs() {
		if yellowLocationID(yellowprofile.GameID, mapID) == location {
			return mapID, true
		}
	}
	return 0, false
}
