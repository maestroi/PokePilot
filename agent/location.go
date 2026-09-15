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
	NativeLocations map[uint8]LocationID
}

func (t KnowledgeTopology) locationForNative(id uint8) LocationID {
	if location := t.NativeLocations[id]; location != "" {
		return location
	}
	return legacyLocationID(id)
}

func legacyLocationID(id uint8) LocationID {
	return LocationID(fmt.Sprintf("legacy-map/%02x", id))
}

func rawLegacyKnowledgeTopology(adjacency map[uint8][]uint8) KnowledgeTopology {
	topology := KnowledgeTopology{
		Adjacency:       map[LocationID][]LocationID{},
		NativeLocations: map[uint8]LocationID{},
	}
	for from, neighbors := range adjacency {
		fromID := legacyLocationID(from)
		topology.NativeLocations[from] = fromID
		for _, native := range neighbors {
			toID := legacyLocationID(native)
			topology.NativeLocations[native] = toID
			topology.Adjacency[fromID] = append(topology.Adjacency[fromID], toID)
		}
	}
	return normalizeKnowledgeTopology(topology)
}

// legacyKnowledgeTopology is the one-release compatibility path for callers
// that still provide only a native byte graph. With exactly one registered
// game adapter, native bytes can be translated without ambiguity and the
// resulting Knowledge remains semantic. If multiple games are registered,
// callers must provide a GameID or a semantic KnowledgeTopology explicitly.
func legacyKnowledgeTopology(adjacency map[uint8][]uint8) KnowledgeTopology {
	if len(knowledgeTopologyProviders) == 1 {
		for _, provider := range knowledgeTopologyProviders {
			return normalizeKnowledgeTopology(provider.KnowledgeTopology(adjacency))
		}
	}
	return rawLegacyKnowledgeTopology(adjacency)
}

func normalizeKnowledgeTopology(t KnowledgeTopology) KnowledgeTopology {
	if t.Adjacency == nil {
		t.Adjacency = map[LocationID][]LocationID{}
	}
	if t.NativeLocations == nil {
		t.NativeLocations = map[uint8]LocationID{}
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
	KnowledgeTopology(map[uint8][]uint8) KnowledgeTopology
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

func knowledgeTopologyFor(id game.GameID, native map[uint8][]uint8) KnowledgeTopology {
	if id != "" {
		if provider := knowledgeTopologyProviders[id]; provider != nil {
			return normalizeKnowledgeTopology(provider.KnowledgeTopology(native))
		}
		return rawLegacyKnowledgeTopology(native)
	}
	return legacyKnowledgeTopology(native)
}

func observationLocation(obs Observation, k *Knowledge) LocationID {
	if obs.Location != "" {
		return LocationID(obs.Location)
	}
	if k != nil {
		return k.locationForNative(obs.Map)
	}
	return legacyLocationID(obs.Map)
}
