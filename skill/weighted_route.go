package skill

import (
	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

// redGlobalRouteCostPolicy keeps global route pricing compatible with the
// local field-path units introduced for speedrun routing: one movement tile is
// one unit, and Cut/Surf/Strength use the same finite action penalties.
func redGlobalRouteCostPolicy(prereqs world.RoutePrerequisites) world.RouteCostPolicy {
	policy := world.DefaultRouteCostPolicy()
	policy.AllowWater = prereqs.Capabilities.Has(capCanSurf)
	policy.CapabilityActionCosts = map[gameruntime.CapabilityID]int{
		capCanCut:          fieldTravelCutActionCost,
		capCanSurf:         fieldTravelSurfActionCost,
		capCanMoveBoulders: fieldTravelStrengthActionCost,
	}
	return policy
}

// routePlanByTravelPolicy is the production seam shared by GoTo and fast
// travel. Speedrun objectives use weighted movement/action cost; other play
// styles retain the established component-aware BFS route and its historical
// transition-count cost so the policy change is opt-in rather than global.
func routePlanByTravelPolicy(
	m *emu.Emu,
	g *world.Graph,
	from, to uint8,
	x, y, tx, ty int,
	blockedHere map[world.Edge]bool,
	prereqs world.RoutePrerequisites,
) (world.RouteCostResult, error) {
	if travelCostPolicyFor(m) == TravelCostFastest {
		return world.FindWeightedRoutePlanAtDestinationWithCapabilities(
			g, from, to, x, y, tx, ty, blockedHere, prereqs,
			redGlobalRouteCostPolicy(prereqs),
		)
	}

	steps, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, from, to, x, y, tx, ty, blockedHere, prereqs,
	)
	if err != nil {
		return world.RouteCostResult{}, err
	}
	cost := len(steps) * fastTravelMapTransitionCost
	if from == to && x >= 0 && y >= 0 && tx >= 0 && ty >= 0 {
		dx := x - tx
		if dx < 0 {
			dx = -dx
		}
		dy := y - ty
		if dy < 0 {
			dy = -dy
		}
		cost += dx + dy
	}
	return world.RouteCostResult{Steps: steps, Cost: cost, Exact: false}, nil
}
