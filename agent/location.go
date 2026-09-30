package agent

import (
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/game"
)

// LocationID is the durable identity of a map/area in generic run knowledge.
// It is deliberately string-backed so bank/group games can use identities such
// as "johto/goldenrod/pokemon-center" without changing core map key types.
type LocationID string

// KnowledgeTopology is rebuilt by the active game adapter each run. Adjacency
// is semantic and durable; NativeLocations exists only at the adapter/runtime
// boundary to migrate old checkpoints and translate transient emulator samples.
type KnowledgeTopology struct {
	Adjacency       map[LocationID][]LocationID
	NativeLocations map[uint16]LocationID
	GameID          game.GameID
}

func (t KnowledgeTopology) locationForNative(id uint16) LocationID {
	if location := t.NativeLocations[id]; location != "" {
		return location
	}
	return locationForNativeGame(t.GameID, id)
}

func rawLegacyLocationID(id uint16) LocationID {
	return LocationID(fmt.Sprintf("legacy-map/%04x", id))
}

// locationForNativeGame resolves a native map id through the provider for the
// named game. When the game is unknown or has no provider, it falls back to
// the legacy all-provider consensus path.
func locationForNativeGame(id game.GameID, native uint16) LocationID {
	if id != "" {
		if provider := knowledgeTopologyProviders[id]; provider != nil {
			if location := provider.KnowledgeTopology(map[uint16][]uint16{native: nil}).NativeLocations[native]; location != "" {
				return location
			}
		}
	}
	return legacyLocationID(native)
}

// legacyLocationID is the compatibility translation for callers that still
// carry only a native byte map. The byte is resolved through the registered
// game adapters only while every one of them maps it to the same semantic
// LocationID: an unidentified observation then stays unambiguous instead of
// silently picking a game. Games that disagree — or no game at all — fall
// back to the raw id, and the caller must identify its game.
func legacyLocationID(id uint16) LocationID {
	var first LocationID
	for _, provider := range knowledgeTopologyProviders {
		location := provider.KnowledgeTopology(map[uint16][]uint16{id: nil}).NativeLocations[id]
		switch {
		case location == "":
			return rawLegacyLocationID(id)
		case first == "":
			first = location
		case location != first:
			return rawLegacyLocationID(id)
		}
	}
	if first == "" {
		return rawLegacyLocationID(id)
	}
	return first
}

func rawLegacyKnowledgeTopology(adjacency map[uint16][]uint16) KnowledgeTopology {
	topology := KnowledgeTopology{
		Adjacency:       map[LocationID][]LocationID{},
		NativeLocations: map[uint16]LocationID{},
	}
	for from, neighbors := range adjacency {
		fromID := rawLegacyLocationID(from)
		topology.NativeLocations[from] = fromID
		for _, native := range neighbors {
			toID := rawLegacyLocationID(native)
			topology.NativeLocations[native] = toID
			topology.Adjacency[fromID] = append(topology.Adjacency[fromID], toID)
		}
	}
	return normalizeKnowledgeTopology(topology)
}

func normalizeKnowledgeTopology(t KnowledgeTopology) KnowledgeTopology {
	if t.Adjacency == nil {
		t.Adjacency = map[LocationID][]LocationID{}
	}
	if t.NativeLocations == nil {
		t.NativeLocations = map[uint16]LocationID{}
	}
	for from, neighbors := range t.Adjacency {
		seen := map[LocationID]bool{}
		out := make([]LocationID, 0, len(neighbors))
		for _, to := range neighbors {
			if to == "" || seen[to] {
				continue
			}
			seen[to] = true
			out = append(out, to)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		t.Adjacency[from] = out
	}
	return t
}

type KnowledgeTopologyProvider interface {
	KnowledgeTopology(map[uint16][]uint16) KnowledgeTopology
}

var knowledgeTopologyProviders = map[game.GameID]KnowledgeTopologyProvider{}

func registerKnowledgeTopologyProvider(id game.GameID, provider KnowledgeTopologyProvider) {
	if id == "" || provider == nil {
		panic("agent: invalid knowledge topology provider")
	}
	if _, exists := knowledgeTopologyProviders[id]; exists {
		panic("agent: duplicate knowledge topology provider for " + string(id))
	}
	knowledgeTopologyProviders[id] = provider
}

// KnowledgeTopologyFor translates an adapter-native map graph into the semantic
// topology accepted by NewKnowledge and checkpoint loading. Callers must name
// the game explicitly; unsupported or empty game IDs stay raw rather than
// guessing from whichever adapters happen to be registered in this process.
func KnowledgeTopologyFor(id game.GameID, native map[uint16][]uint16) KnowledgeTopology {
	if provider := knowledgeTopologyProviders[id]; provider != nil {
		topology := normalizeKnowledgeTopology(provider.KnowledgeTopology(native))
		topology.GameID = id
		return topology
	}
	return rawLegacyKnowledgeTopology(native)
}

func knowledgeTopologyFor(id game.GameID, native map[uint16][]uint16) KnowledgeTopology {
	return KnowledgeTopologyFor(id, native)
}

func observationLocation(obs Observation, k *Knowledge) LocationID {
	if obs.Location != "" {
		return LocationID(obs.Location)
	}
	if k != nil {
		return k.locationForNative(uint16(obs.Map))
	}
	return locationForNativeGame(obs.GameID, uint16(obs.Map))
}
