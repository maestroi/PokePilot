package agent

import "github.com/maestroi/pokepilot/game"

// ObjectiveCatalogProvider is implemented by a concrete game adapter. It is
// intentionally separate from ProgressionPlanner so games can expose world
// opportunities without coupling catalog discovery to story sequencing.
type ObjectiveCatalogProvider interface {
	ObjectiveCatalog(Observation) ObjectiveCatalog
}

var objectiveCatalogProviders = map[game.GameID]ObjectiveCatalogProvider{}

func registerObjectiveCatalogProvider(id game.GameID, provider ObjectiveCatalogProvider) {
	if id == "" || provider == nil {
		panic("agent: invalid objective catalog provider")
	}
	if _, exists := objectiveCatalogProviders[id]; exists {
		panic("agent: duplicate objective catalog provider for " + string(id))
	}
	objectiveCatalogProviders[id] = provider
}

func objectiveCatalogProviderFor(gameID game.GameID) (ObjectiveCatalogProvider, bool) {
	if gameID != "" {
		provider, ok := objectiveCatalogProviders[gameID]
		return provider, ok
	}
	// One-release compatibility for synthetic callers that predate GameID on
	// Observation. With exactly one registered game, there is no ambiguity. As
	// soon as another game is registered, synthetic observations must identify
	// their game explicitly rather than silently selecting one.
	if len(objectiveCatalogProviders) != 1 {
		return nil, false
	}
	for _, provider := range objectiveCatalogProviders {
		return provider, true
	}
	return nil, false
}
