package agent

import (
	"slices"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestRedRouteAvailabilityIncludesInteractionPlacesWithoutExposingGenericJourney(t *testing.T) {
	for _, place := range []string{
		"vermilion old rod house",
		skill.FossilRevivalPlace(),
	} {
		if slices.Contains(skill.PlaceNames(), place) {
			t.Fatalf("%q became a generic journey; compound destinations must stay interaction-owned", place)
		}
		if !slices.Contains(redRouteAvailabilityPlaceNames(), place) {
			t.Fatalf("route availability omitted interaction destination %q", place)
		}
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

func TestRedRouteAvailabilityProjectsBlockedFossilRevival(t *testing.T) {
	place := skill.FossilRevivalPlace()
	dest, ok := skill.Place(place)
	if !ok {
		t.Fatalf("missing interaction destination %q", place)
	}
	planner := fakeRouteReachability{
		dest: blockedRoute("red:route21_surf", "can_surf"),
	}
	got := collectRouteAvailability(planner, redRouteAvailabilityPlaceNames(), redRoutePrerequisiteLink)
	if !slices.Contains(got.Unroutable, place) {
		t.Fatalf("unroutable = %v; want fossil revival withheld behind Surf", got.Unroutable)
	}
	for _, blockage := range got.Blockages {
		if blockage.Destination != PlaceID(place) {
			continue
		}
		if len(blockage.Missing) != 1 || blockage.Missing[0] != "can_surf" {
			t.Fatalf("fossil revival blockage = %+v; want can_surf prerequisite", blockage)
		}
		return
	}
	t.Fatalf("no structured blockage projected for %q: %+v", place, got.Blockages)
}
