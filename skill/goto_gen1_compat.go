package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

func gen1GoToCompatibilityEnabled(m *emu.Emu) bool {
	if m == nil {
		return false
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return false
	}
	switch string(profile.ID()) {
	case "pokemon-red", "pokemon-blue", "pokemon-yellow":
		return true
	default:
		return false
	}
}

// applyGoToCompatibilityInitialTopology keeps Kanto-only route overlays out of
// the reusable GoTo driver. Future non-Gen-I profiles get the provider graph
// unchanged and can supply their own story/progression semantics separately.
func applyGoToCompatibilityInitialTopology(m *emu.Emu, g *world.Graph, romData []byte) (*world.Graph, error) {
	if !gen1GoToCompatibilityEnabled(m) {
		return g, nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	return withAsleepRoute16Snorlax(g, romData, &mem)
}

// applyGoToCompatibilityLocalTopology applies the remaining live Kanto
// Route-16 corridor facts only for Gen-I profiles.
func applyGoToCompatibilityLocalTopology(
	m *emu.Emu,
	cur uint8,
	blockers map[[2]int]bool,
	grid *world.Grid,
	romData []byte,
) map[[2]int]bool {
	if !gen1GoToCompatibilityEnabled(m) || cur != route16Map {
		return blockers
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, eventBeatRoute16Snorlax) {
		if blockers == nil {
			blockers = map[[2]int]bool{}
		}
		blockers[[2]int{route16SnorlaxX, route16SnorlaxY}] = true
	}
	openRoute16CutPassage(grid, romData, &mem)
	return blockers
}

// goToRoutePrerequisites returns Kanto's semantic route gates for Gen-I
// profiles. Other generations start with no Red prerequisites; their adapter
// may define generation-owned route semantics without changing GoTo.
func goToRoutePrerequisites(m *emu.Emu, g *world.Graph, romData []byte) world.RoutePrerequisites {
	if !gen1GoToCompatibilityEnabled(m) {
		return world.RoutePrerequisites{}
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	return redRoutePrerequisites(g, romData, &mem)
}
