package agent

import "encoding/json"

// MarshalJSON keeps Observation's runtime route-blockage set complete while
// bounding only the planner-facing representation. Offer consumes Observation
// directly, so dropping semantic blockers during route collection can make a
// progression-locked destination leak back into the menu. The model does not
// need every duplicate downstream destination, though, so the prompt still
// carries at most routeBlockageCap detailed examples.
func (o Observation) MarshalJSON() ([]byte, error) {
	type observationAlias Observation
	copy := observationAlias(o)
	if len(copy.RouteBlockages) > routeBlockageCap {
		copy.RouteBlockages = append([]RouteBlockage(nil), copy.RouteBlockages[:routeBlockageCap]...)
	}
	return json.Marshal(copy)
}
