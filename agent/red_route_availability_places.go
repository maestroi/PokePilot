package agent

import "github.com/maestroi/pokepilot/skill"

// redRouteAvailabilityPlaceNames is the set of semantic destinations whose
// live reachability can gate an offered Red objective. Generic PlaceNames stays
// intentionally limited to standalone journey targets; rewards, gifts, trades,
// fossil revival and other compound actions keep their targets interaction-owned.
// They can still be offered as objectives, so the capability-aware route audit
// must include them before they reach the planner.
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
