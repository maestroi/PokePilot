package agent

import (
	"strings"
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

func redOffered(obs Observation, known *Knowledge) []Objective {
	return OfferWithProgression(obs, known, &redObjectiveAdapter{})
}

func TestOfferWithholdsVermilionGymJourneyUntilInside(t *testing.T) {
	known := NewKnowledge(nil)
	known.Visited[LocationID("vermilion gym")] = true

	obs := Observation{GameID: testGameID, Map: 0x05, MapName: "VERMILION_CITY", PartyCount: 1}
	if hasJourneyTo(redOffered(obs, known), "vermilion gym") {
		t.Fatal("Vermilion City offered a plain journey to the gym before entering it")
	}

	obs = Observation{GameID: testGameID, Map: 0x5c, Location: "vermilion gym", MapName: "VERMILION_GYM", PartyCount: 1}
	if !hasJourneyTo(redOffered(obs, known), "vermilion gym") {
		t.Fatal("already inside the gym, the journey to its own stand tile must still be offered")
	}
}

func TestOfferPewterCitySurfacesBrockUntilBoulderBadge(t *testing.T) {
	obs := Observation{GameID: testGameID, Map: 0x02, MapName: "PEWTER_CITY", PartyCount: 1}
	known := NewKnowledge(nil)
	if !hasOfferedKind(redOffered(obs, known), KindGym) {
		t.Fatal("Pewter City did not offer the Brock gym challenge before the Boulder Badge")
	}

	obs.Badges = []string{state.BadgeBoulder.String()}
	if hasOfferedKind(redOffered(obs, known), KindGym) {
		t.Fatal("Pewter City still offered Brock after the Boulder Badge was observed")
	}
}

func TestOfferWithholdsGymWhenDestinationIsSemanticallyBlocked(t *testing.T) {
	obs := Observation{GameID: testGameID,
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
	obs := Observation{GameID: testGameID, Map: 0x05, MapName: "VERMILION_CITY", PartyCount: 1}
	known := NewKnowledge(nil)
	if !hasOfferedKind(Offer(obs, known), KindGym) {
		t.Fatal("empty RouteBlockages withheld the Vermilion gym; Offer must fail open")
	}
}

func TestOfferGymDoneCountNeverCarriesOverFromAnotherGym(t *testing.T) {
	known := NewKnowledge(nil)
	known.Done(Objective{Kind: KindGym, Place: "pewter city"})
	known.Done(Objective{Kind: KindGym, Place: "cerulean city"})

	// This test is intentionally about generic objective identity annotation,
	// not Red's compound Cut-owned entry gate, so use portable Offer directly.
	obs := Observation{GameID: testGameID, Map: 0x05, MapName: "VERMILION_CITY", PartyCount: 1}
	for _, o := range Offer(obs, known) {
		if o.Kind != KindGym {
			continue
		}
		if strings.Contains(o.Note, "done") {
			t.Fatalf("Vermilion gym objective falsely claims prior completion: note=%q", o.Note)
		}
		return
	}
	t.Fatal("Vermilion City did not offer the gym challenge")
}

func TestOfferKeepsGymWhenAlreadyInsideABlockedGymMap(t *testing.T) {
	obs := Observation{GameID: testGameID,
		Map: 0x5C, Location: "vermilion gym", MapName: "VERMILION_GYM", PartyCount: 1,
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
