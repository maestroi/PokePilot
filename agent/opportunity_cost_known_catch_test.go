package agent

import "testing"

func TestOpportunityCostKnownHabitatCatchIncludesTravel(t *testing.T) {
	profile := PlayStyle("adventure")
	obs := Observation{Map: 0xff, PartyCount: 2}

	local := Objective{Kind: KindCatch, Species: SpeciesID("pidgey")}
	remote := Objective{
		Kind:    KindCatch,
		Species: SpeciesID("pidgey"),
		Place:   PlaceID("viridian city"),
		Flee:    true,
		Note:    "(known habitat: VIRIDIAN CITY; travel included)",
	}

	localCost := opportunityCost(obs, local, profile)
	remoteCost := opportunityCost(obs, remote, profile)
	if !remoteCost.CrossMap {
		t.Fatalf("known-habitat catch was not recognized as cross-map: %#v", remoteCost)
	}
	if remoteCost.Travel <= localCost.Travel {
		t.Fatalf("remote catch travel %.3f <= local catch travel %.3f", remoteCost.Travel, localCost.Travel)
	}
	if remoteCost.Total <= localCost.Total {
		t.Fatalf("remote catch total cost %.3f <= local catch total cost %.3f", remoteCost.Total, localCost.Total)
	}
}
