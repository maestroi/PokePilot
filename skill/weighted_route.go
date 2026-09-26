package skill

import (
	"errors"

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
	if err != nil && !errors.Is(err, world.ErrRouteReplanRequired) {
		return world.RouteCostResult{}, err
	}

	// Normal-play routing deliberately keeps BFS semantics rather than adopting
	// speedrun cost ordering. One legality check is still required: Surf's
	// PortBypass may waive the LAND-component canExit test, but it must not
	// waive the concrete local approach to the selected shore. Route 20 has
	// disconnected Seafoam-side components; BFS could select the opposite
	// shore and Traverse would then fail forever with leg_unwalkable (#1954,
	// same root as #1947).
	//
	// Validate only Surf-owned PortBypass first hops. Other PortBypass actions
	// (for example a Cut tree) intentionally create local land connectivity and
	// cannot be judged from pristine geometry. When the selected Surf port is
	// locally impossible, reuse the existing exact water-aware planner to pick
	// an executable route; otherwise preserve the original BFS route unchanged.
	if firstStepNeedsSurfPortValidation(steps) {
		policy := redGlobalRouteCostPolicy(prereqs)
		if !world.EdgePortReachableFrom(g, from, x, y, steps[0].Edge, policy) {
			exact, exactErr := world.FindWeightedRoutePlanAtDestinationWithCapabilities(
				g, from, to, x, y, tx, ty, blockedHere, prereqs, policy,
			)
			if exact.Exact {
				return exact, exactErr
			}
		}
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
	return world.RouteCostResult{Steps: steps, Cost: cost, Exact: false}, err
}

func firstStepNeedsSurfPortValidation(steps []world.RouteStep) bool {
	if len(steps) == 0 || steps[0].Transition == nil || !steps[0].Transition.PortBypass {
		return false
	}
	for _, capability := range steps[0].Transition.Requires {
		if capability == capCanSurf {
			return true
		}
	}
	return false
}
