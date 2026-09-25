package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// enterVermilionGymViaRouteGate uses the same semantic Cut transition that
// ordinary route execution uses. This keeps the atomic gym objective from
// maintaining a second, weaker tree-search implementation that can drift from
// route semantics or probe unrelated solid tiles near the gym.
func enterVermilionGymViaRouteGate(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != vermilionCity {
		return fmt.Errorf("skill: Vermilion Gym entry: on map %#04x, want %#04x", m.Peek8(sym.CurMap), vermilionCity)
	}
	if policy == nil {
		return fmt.Errorf("skill: Vermilion Gym entry: nil policy")
	}
	if err := RepairUtilityFieldCapability(m, romData, policy, FieldCut); err != nil {
		return fmt.Errorf("skill: Vermilion Gym entry: prepare Cut carrier: %w", err)
	}
	// Carrier repair may leave town to catch a wild Cut carrier (e.g. Oddish on
	// Route 24) and returns wherever the catch ended. The gym door edge starts
	// in Vermilion, so walk back before traversing it.
	if m.Peek8(sym.CurMap) != vermilionCity {
		city, ok := Place("vermilion city")
		if !ok {
			return fmt.Errorf("skill: Vermilion Gym entry: vermilion city place missing")
		}
		if _, err := TravelFlee(m, romData, city, policy, surgeProgressionTravelEngagements); err != nil {
			return fmt.Errorf("skill: Vermilion Gym entry: return to Vermilion after Cut carrier repair: %w", err)
		}
	}

	g, err := world.BuildGraph(romData)
	if err != nil {
		return fmt.Errorf("skill: Vermilion Gym entry: build route graph: %w", err)
	}

	for _, edge := range g.Edges[vermilionCity] {
		if edge.Kind != world.EdgeWarp || edge.To != vermilionGymMap {
			continue
		}
		transition, ok := redRouteTransitionForEdge(edge)
		if !ok || transition.ID != "red:vermilion_gym_cut" {
			continue
		}
		if _, err := world.ExecuteTransition(newRedRouteTransitionExecutor(m, romData, policy), edge, transition); err != nil {
			return fmt.Errorf("skill: Vermilion Gym entry: execute Cut gate: %w", err)
		}
		if err := Traverse(m, romData, edge); err != nil {
			return fmt.Errorf("skill: Vermilion Gym entry: cross gym door after Cut: %w", err)
		}
		if got := m.Peek8(sym.CurMap); got != vermilionGymMap {
			return fmt.Errorf("skill: Vermilion Gym entry: arrived on map %#04x, want %#04x", got, vermilionGymMap)
		}
		return nil
	}

	return fmt.Errorf("skill: Vermilion Gym entry: no Cut-gated warp from map %#04x to %#04x", vermilionCity, vermilionGymMap)
}
