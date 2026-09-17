package agent

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

// MapGraphProvider owns the map-level travel graph for one game. The graph is
// built from a game's own ROM tables, which differ between Gen I games even
// though the header/object format is shared: Yellow's table addresses and its
// valid-map set are both its own. Generic run code must not hardcode one
// game's tables, so a game that needs anything other than Red's registers a
// provider here and run.go asks for it.
type MapGraphProvider interface {
	// BuildGraph returns the map-level graph for romData, or an error the
	// caller surfaces as a run failure.
	BuildGraph(romData []byte) (*world.Graph, error)
}

var mapGraphProviders = map[game.GameID]MapGraphProvider{}

func registerMapGraphProvider(id game.GameID, provider MapGraphProvider) {
	if id == "" || provider == nil {
		panic("agent: invalid map graph provider")
	}
	if _, exists := mapGraphProviders[id]; exists {
		panic("agent: duplicate map graph provider for " + string(id))
	}
	mapGraphProviders[id] = provider
}

// buildMapGraph builds the travel graph for the detected profile. A profile
// without a registered provider gets Red's tables, which is the historical
// default and correct for every Gen I game that shares Red's ROM layout.
func buildMapGraph(profile game.GameProfile, romData []byte) (*world.Graph, error) {
	if profile != nil {
		if provider := mapGraphProviders[profile.ID()]; provider != nil {
			return provider.BuildGraph(romData)
		}
	}
	return world.BuildGraph(romData)
}
