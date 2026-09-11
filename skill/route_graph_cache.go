package skill

import (
	"sync"

	"github.com/maestroi/pokepilot/world"
)

// A loaded ROM is immutable for the lifetime of a run. Keep the expensive
// map-level graph beside that ROM instance instead of rebuilding all maps,
// components, warps, and reachability on every planner observation.
//
// The pointer is safe as an identity key because the map key itself retains a
// pointer into the backing allocation. Tests/other callers using distinct ROM
// slices still get distinct graphs even when their bytes happen to match.
type routeGraphROMKey struct {
	first  *byte
	length int
}

type routeGraphCacheEntry struct {
	graph *world.Graph
	err   error
}

var routeGraphCache = struct {
	sync.Mutex
	entries map[routeGraphROMKey]routeGraphCacheEntry
}{entries: make(map[routeGraphROMKey]routeGraphCacheEntry)}

func cachedRouteGraph(romData []byte) (*world.Graph, error) {
	if len(romData) == 0 {
		return world.BuildGraph(romData)
	}
	key := routeGraphROMKey{first: &romData[0], length: len(romData)}
	routeGraphCache.Lock()
	defer routeGraphCache.Unlock()
	if cached, ok := routeGraphCache.entries[key]; ok {
		return cached.graph, cached.err
	}
	graph, err := world.BuildGraph(romData)
	routeGraphCache.entries[key] = routeGraphCacheEntry{graph: graph, err: err}
	return graph, err
}
