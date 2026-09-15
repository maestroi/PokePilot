package agent

import (
	"slices"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestRedRouteAvailabilityIncludesChoiceRewardsWithoutExposingGenericJourney(t *testing.T) {
	const oldRodPlace = "vermilion old rod house"
	if slices.Contains(skill.PlaceNames(), oldRodPlace) {
		t.Fatalf("%q became a generic journey; scripted reward destinations must stay interaction-owned", oldRodPlace)
	}
	if !slices.Contains(redRouteAvailabilityPlaceNames(), oldRodPlace) {
		t.Fatalf("route availability omitted scripted reward destination %q", oldRodPlace)
	}
}

func TestRedRouteAvailabilityProjectsBlockedChoiceReward(t *testing.T) {
	const oldRodPlace = "vermilion old rod house"
	dest, ok := skill.Place(oldRodPlace)
	if !ok {
		t.Fatalf("missing interaction destination %q", oldRodPlace)
	}
	planner := fakeRouteReachability{
		dest: blockedRoute("red:route21_surf", "can_surf"),
	}
	got := collectRouteAvailability(planner, redRouteAvailabilityPlaceNames(), redRoutePrerequisiteLink)
	if !slices.Contains(got.Unroutable, oldRodPlace) {
		t.Fatalf("unroutable = %v; want %q withheld before it can be offered", got.Unroutable, oldRodPlace)
	}
	for _, blockage := range got.Blockages {
		if blockage.Destination != PlaceID(oldRodPlace) {
			continue
		}
		if len(blockage.Missing) != 1 || blockage.Missing[0] != "can_surf" {
			t.Fatalf("old rod blockage = %+v; want can_surf prerequisite", blockage)
		}
		return
	}
	t.Fatalf("no structured blockage projected for %q: %+v", oldRodPlace, got.Blockages)
}
