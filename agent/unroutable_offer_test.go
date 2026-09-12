package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestOfferDoesNotFailOpenWhenEveryKnownJourneyIsUnroutable(t *testing.T) {
	const route12 = uint8(0x17)
	adjacency := map[uint8][]uint8{route12: {}}
	for _, name := range []string{"cerulean city", "lavender town", "route 9", "route 22"} {
		d, ok := skill.Place(name)
		if !ok {
			t.Fatalf("missing place fixture %q", name)
		}
		adjacency[route12] = append(adjacency[route12], d.Map)
		adjacency[d.Map] = append(adjacency[d.Map], route12)
	}
	known := NewKnowledge(adjacency)
	known.SawMap(route12)

	obs := Observation{
		Map:        route12,
		MapName:    "ROUTE_12",
		X:          9,
		Y:          62,
		PartyCount: 1,
	}
	baselineHasJourney := false
	for _, o := range Offer(obs, known) {
		if o.Kind == KindGoTo {
			baselineHasJourney = true
			break
		}
	}
	if !baselineHasJourney {
		t.Fatal("test fixture has no journey before reachability filtering")
	}

	// routeAvailabilityFor returns an empty slice when route analysis itself
	// failed, so a non-empty Unroutable list is positive evidence from a
	// successful live route snapshot. Marking every named place unavailable
	// must therefore be allowed to reduce the journey menu to zero.
	obs.Unroutable = append([]string(nil), skill.PlaceNames()...)
	for _, o := range Offer(obs, known) {
		if o.Kind == KindGoTo {
			t.Fatalf("all-unroutable Route 12 state re-offered impossible journey %q", o)
		}
	}
}
