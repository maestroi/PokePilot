package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

// A semantic route blocker is runtime policy, not merely prompt decoration.
// Even a destination whose detailed blockage falls after routeBlockageCap must
// stay withheld from Offer; only the JSON projection is allowed to truncate.
func TestSemanticRouteBlockerBeyondPromptCapStillFiltersOffer(t *testing.T) {
	const target = "viridian city"
	targetDest, ok := skill.Place(target)
	if !ok {
		t.Fatal("missing viridian city fixture")
	}

	names := make([]string, 0, routeBlockageCap+1)
	planner := fakeRouteReachability{}
	for _, name := range skill.PlaceNames() { // already sorted
		if name == target {
			break
		}
		d, ok := skill.Place(name)
		if !ok {
			continue
		}
		names = append(names, name)
		planner[d] = blockedRoute("portable:gate", "can_surf")
		if len(names) == routeBlockageCap {
			break
		}
	}
	if len(names) != routeBlockageCap {
		t.Fatalf("need %d earlier place fixtures before %q, got %d", routeBlockageCap, target, len(names))
	}
	names = append(names, target)
	planner[targetDest] = blockedRoute("portable:gate", "can_surf")

	known := NewKnowledge(map[uint8][]uint8{targetDest.Map: {targetDest.Map}})
	known.SawMap(targetDest.Map)
	base := Observation{Map: targetDest.Map, MapName: "VIRIDIAN_CITY", X: 0, Y: 0, PartyCount: 1}
	if !offersPlace(Offer(base, known), target) {
		t.Fatalf("test fixture invalid: %q is not offered before route availability is applied", target)
	}

	availability := collectRouteAvailability(planner, names)
	if len(availability.Blockages) != routeBlockageCap+1 {
		t.Fatalf("runtime retained %d semantic blockages, want %d", len(availability.Blockages), routeBlockageCap+1)
	}
	base.Unroutable = availability.Unroutable
	base.RouteBlockages = availability.Blockages
	if offersPlace(Offer(base, known), target) {
		t.Fatalf("semantic blocker after prompt cap leaked %q back into Offer", target)
	}
}
