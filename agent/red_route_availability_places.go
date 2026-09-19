package agent

import "github.com/maestroi/pokepilot/skill"

// redRouteAvailabilityPlaceNames is the set of semantic destinations whose
// live reachability can gate an offered Red objective. Generic PlaceNames stays
// intentionally limited to standalone journey targets; scripted NPC rewards
// are interaction-owned destinations, but appendRedNPCRewardObjectives can
// still offer a journey to them and therefore needs the same capability-aware
// route filtering before they reach the planner.
func redRouteAvailabilityPlaceNames() []string {
	names := append([]string(nil), skill.PlaceNames()...)
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		seen[name] = true
	}
	for _, name := range skill.InteractionPlaceNames() {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}
