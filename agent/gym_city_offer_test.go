package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func hasOfferedKind(objs []Objective, kind Kind) bool {
	for _, o := range objs {
		if o.Kind == kind {
			return true
		}
	}
	return false
}

func TestOfferPewterCitySurfacesBrockUntilBoulderBadge(t *testing.T) {
	obs := Observation{Map: 0x02, MapName: "PEWTER_CITY", PartyCount: 1}
	known := NewKnowledge(nil)
	if !hasOfferedKind(Offer(obs, known), KindGym) {
		t.Fatal("Pewter City did not offer the Brock gym challenge before the Boulder Badge")
	}

	obs.Badges = []string{state.BadgeBoulder.String()}
	if hasOfferedKind(Offer(obs, known), KindGym) {
		t.Fatal("Pewter City still offered Brock after the Boulder Badge was observed")
	}
}

// TestOfferWithholdsGymWhenDestinationIsSemanticallyBlocked: farm run
// run-1m8sj0ew30bal34g5p3273dic0 offered "beat the gym leader here" from
// Vermilion City while RouteBlockages already said vermilion gym was missing
// can_cut. KindGym is not a journey, so the existing place filter did not
// apply; the planner copied the illegal verb into a plan and Gym died on
// the Cut badge check. Deterministic code owns legality — do not offer a
// gym whose interior is gated from here.
func TestOfferWithholdsGymWhenDestinationIsSemanticallyBlocked(t *testing.T) {
	obs := Observation{
		Map: 0x05, MapName: "VERMILION_CITY", PartyCount: 1,
		RouteBlockages: []RouteBlockage{{
			Destination: "vermilion gym",
			Missing:     []CapabilityID{"can_cut"},
		}},
	}
	known := NewKnowledge(nil)
	if hasOfferedKind(Offer(obs, known), KindGym) {
		t.Fatal("Vermilion City offered the gym while its destination is missing can_cut")
	}
}

func TestOfferStillSurfacesGymWhenRouteBlockagesOmitIt(t *testing.T) {
	obs := Observation{Map: 0x05, MapName: "VERMILION_CITY", PartyCount: 1}
	known := NewKnowledge(nil)
	if !hasOfferedKind(Offer(obs, known), KindGym) {
		t.Fatal("empty RouteBlockages withheld the Vermilion gym; Offer must fail open")
	}
}

func TestOfferKeepsGymWhenAlreadyInsideABlockedGymMap(t *testing.T) {
	obs := Observation{
		Map: 0x5C, MapName: "VERMILION_GYM", PartyCount: 1,
		RouteBlockages: []RouteBlockage{{
			Destination: "vermilion gym",
			Missing:     []CapabilityID{"can_cut"},
		}},
	}
	known := NewKnowledge(nil)
	if !hasOfferedKind(Offer(obs, known), KindGym) {
		t.Fatal("already inside Vermilion Gym, the Cut tree is behind the player; withholding the challenge traps them")
	}
}
