package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

// gsKnowledgeTopologyProvider translates the wide-id Gold/Silver native map
// adjacency into the semantic topology. Gen II identifies a map by (group,
// number), a 16-bit identity that the historical uint8 world graph cannot
// carry, so the provider consumes the portable uint16 adjacency directly.
type gsKnowledgeTopologyProvider struct {
	gameID game.GameID
}

func init() {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		registerKnowledgeTopologyProvider(id, gsKnowledgeTopologyProvider{gameID: id})
	}
}

func gsLocationID(id game.GameID, native uint16) LocationID {
	if info, ok := gsdata.MapByNative(native); ok && info.Location != "" {
		return LocationID(semanticLocation(string(info.Location)))
	}
	return LocationID(fmt.Sprintf("%s/map/%04x", id, native))
}

func (p gsKnowledgeTopologyProvider) KnowledgeTopology(native map[uint16][]uint16) KnowledgeTopology {
	topology := KnowledgeTopology{
		Adjacency:       map[LocationID][]LocationID{},
		NativeLocations: map[uint16]LocationID{},
	}
	location := func(id uint16) LocationID {
		if known := topology.NativeLocations[id]; known != "" {
			return known
		}
		semantic := gsLocationID(p.gameID, id)
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
