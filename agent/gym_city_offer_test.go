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

func hasJourneyTo(objs []Objective, place PlaceID) bool {
	for _, o := range objs {
		if o.Kind == KindGoTo && o.Place == place {
			return true
		}
	}
	return false
}

// TestOfferWithholdsVermilionGymJourneyUntilInside: farm run
// run-2p2b5kf4qza0o1cv5vo5swhzxr offered "go to vermilion gym" as a plain
// journey while Cut was already learned and usable, so RouteBlockages never
// flagged it. GoTo/Traverse has no walkable edge onto the gym map either
// way — the door is behind a Cut tree only EnterVermilionGym (invoked by
// the KindGym objective) knows how to clear — so GoTo oscillated between
// Vermilion City's neighboring routes until it gave up with
// ErrNavigationStalled. The journey must stay withheld until the player is
// already standing on the gym map.
func TestOfferWithholdsVermilionGymJourneyUntilInside(t *testing.T) {
	known := NewKnowledge(nil)
	known.Visited[0x5c] = true

	obs := Observation{Map: 0x05, MapName: "VERMILION_CITY", PartyCount: 1}
	if hasJourneyTo(Offer(obs, known), "vermilion gym") {
		t.Fatal("Vermilion City offered a plain journey to the gym before entering it")
	}

	obs = Observation{Map: 0x5c, MapName: "VERMILION_GYM", PartyCount: 1}
	if !hasJourneyTo(Offer(obs, known), "vermilion gym") {
		t.Fatal("already inside the gym, the journey to its own stand tile must still be offered")
	}
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
