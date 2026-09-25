package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

// routePlannerGen1Compatibility keeps the remaining Gen-I story/capability
// overlay out of reusable routable.go. Static/live geometry is already
// profile-semantic; these event/badge/HM encodings are still owned by the
// existing Gen-I route semantics until #972/#973 provide another game's
// progression adapter.
func routePlannerGen1Compatibility(
	m *emu.Emu,
	romData []byte,
	base *world.Graph,
	cur uint8,
	liveGrid *world.Grid,
) (*world.Graph, world.RoutePrerequisites, error) {
	if m == nil || base == nil || liveGrid == nil {
		return nil, world.RoutePrerequisites{}, fmt.Errorf("skill: RoutePlanner Gen-I compatibility: incomplete routing input")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if cur == route16Map {
		if !state.HasEvent(&mem, eventBeatRoute16Snorlax) {
			liveGrid.Set(route16SnorlaxX, route16SnorlaxY, false)
		}
		openRoute16CutPassage(liveGrid, romData, &mem)
	}

	routeGraph, err := base.WithMapGrid(cur, liveGrid)
	if err != nil {
		return nil, world.RoutePrerequisites{}, err
	}
	if cur != route16Map {
		routeGraph, err = withAsleepRoute16Snorlax(routeGraph, romData, &mem)
		if err != nil {
			return nil, world.RoutePrerequisites{}, err
		}
	}
	return routeGraph, redRoutePrerequisites(routeGraph, romData, &mem), nil
}
