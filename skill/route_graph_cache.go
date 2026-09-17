package skill

import (
	"sync"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
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
	return cachedRouteGraphForTables(graphForROM(romData), romData)
}

// graphForROM resolves which map-table set a ROM image needs. A ROM is
// immutable for the lifetime of a run, and the cache key retains a pointer
// into the backing allocation, so one image always resolves one table set.
func graphForROM(romData []byte) rom.Tables {
	if isYellowROM(romData) {
		return yellowrom.Tables()
	}
	return rom.RedTables()
}

// cachedRouteGraphForTables is cachedRouteGraph with an explicit table set.
func cachedRouteGraphForTables(tables rom.Tables, romData []byte) (*world.Graph, error) {
	if len(romData) == 0 {
		return world.BuildGraphForTables(tables, romData)
	}
	key := routeGraphROMKey{first: &romData[0], length: len(romData)}
	routeGraphCache.Lock()
	defer routeGraphCache.Unlock()
	if cached, ok := routeGraphCache.entries[key]; ok {
		return cached.graph, cached.err
	}
	graph, err := world.BuildGraphForTables(tables, romData)
	routeGraphCache.entries[key] = routeGraphCacheEntry{graph: graph, err: err}
	return graph, err
}
