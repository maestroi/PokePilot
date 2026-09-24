package agent

import "testing"

func recoveryTestKnowledge(edges map[LocationID][]LocationID, visited ...LocationID) *Knowledge {
	k := NewKnowledge(KnowledgeTopology{Adjacency: edges})
	for _, location := range visited {
		k.SawLocation(location)
	}
	return k
}

func TestRecoveryRankingUsesReturnToInterruptedObjective(t *testing.T) {
	current := LocationID("current")
	centerA := LocationID("center-a")
	centerB := LocationID("center-b")
	gym := LocationID("gym")
	known := recoveryTestKnowledge(map[LocationID][]LocationID{
		current: {centerA, centerB},
		centerA: {current},
		centerB: {current, gym},
		gym:     {centerB},
	}, current, centerA, centerB)

	challenge := Objective{Kind: KindGym, Place: "test gym"}
	known.Failures[combatLossFailureKey(challenge)] = Failure{
		Objective: challenge.String(), Times: 1, ReadinessTarget: 100,
	}
	obs := Observation{Location: PlaceID(current)}
	catalog := ObjectiveCatalog{
		Challenges: []CatalogChallenge{{Place: "test gym", Location: gym}},
		Destinations: []CatalogDestination{
			{Place: "center a", Location: centerA, Center: true, TravelCostKnown: true, TravelCostChecked: true, TravelCost: 100},
			{Place: "center b", Location: centerB, Center: true, TravelCostKnown: true, TravelCostChecked: true, TravelCost: 200},
		},
	}
	ranked := rankRecoveryCheckpoints(obs, known, map[LocationID]bool{current: true, centerA: true, centerB: true}, catalog, nil)
	if len(ranked) != 2 {
		t.Fatalf("ranking = %+v, want two candidates", ranked)
	}
	if ranked[0].Place != "center b" || !ranked[0].Selected {
		t.Fatalf("best = %+v, want center b because return-to-gym is cheaper", ranked[0])
	}
	if ranked[0].ReturnCost >= ranked[1].ReturnCost {
		t.Fatalf("return costs = %+v, want center b return cost lower", ranked)
	}
}

func TestRecoveryRankingKeepsActiveCheckpointOnComparableCost(t *testing.T) {
	current := LocationID("current")
	near := LocationID("near")
	active := LocationID("active")
	known := recoveryTestKnowledge(map[LocationID][]LocationID{
		current: {near},
		near:    {current, active},
		active:  {near},
	}, current, near, active)
	obs := Observation{Location: PlaceID(current), RecoveryCheckpoint: "active center"}
	catalog := ObjectiveCatalog{Destinations: []CatalogDestination{
		{Place: "near center", Location: near, Center: true},
		{Place: "active center", Location: active, Center: true},
	}}
	ranked := rankRecoveryCheckpoints(obs, known, map[LocationID]bool{current: true, near: true, active: true}, catalog, nil)
	if len(ranked) != 2 || ranked[0].Place != "active center" || !ranked[0].Selected {
		t.Fatalf("ranking = %+v, want active checkpoint to win a one-hop comparable difference", ranked)
	}
}

func TestRecoveryRankingCanPreferFartherCenterViaFastTravel(t *testing.T) {
	current := LocationID("current")
	near := LocationID("near")
	far := LocationID("far")
	known := recoveryTestKnowledge(map[LocationID][]LocationID{
		current: {near},
		near:    {current, far},
		far:     {near},
	}, current, near, far)
	obs := Observation{Location: PlaceID(current)}
	catalog := ObjectiveCatalog{Destinations: []CatalogDestination{
		{Place: "near center", Location: near, Center: true, TravelCostChecked: true, TravelCostKnown: true, TravelCost: 100},
		{Place: "far center", Location: far, Center: true, TravelCostChecked: true, TravelCostKnown: true, TravelCost: 40, FastTravel: true, FastTravelMethod: "fly"},
	}}
	ranked := rankRecoveryCheckpoints(obs, known, map[LocationID]bool{current: true, near: true, far: true}, catalog, nil)
	if len(ranked) != 2 || ranked[0].Place != "far center" || !ranked[0].FastTravel {
		t.Fatalf("ranking = %+v, want farther Center to win through legal Fly cost", ranked)
	}
}

func TestRecoveryRankingRejectsCapabilityBlockedCenter(t *testing.T) {
	current := LocationID("current")
	blocked := LocationID("blocked")
	known := recoveryTestKnowledge(map[LocationID][]LocationID{
		current: {blocked},
		blocked: {current},
	}, current, blocked)
	obs := Observation{Location: PlaceID(current)}
	catalog := ObjectiveCatalog{Destinations: []CatalogDestination{{
		Place: "blocked center", Location: blocked, Center: true,
		TravelCostChecked: true, TravelCostKnown: false,
	}}}
	ranked := rankRecoveryCheckpoints(obs, known, map[LocationID]bool{current: true, blocked: true}, catalog, nil)
	if len(ranked) != 1 || ranked[0].Routable || ranked[0].Selected {
		t.Fatalf("ranking = %+v, want checked-but-unreachable Center rejected", ranked)
	}
}

func TestRecoveryRankingTieBreaksDeterministically(t *testing.T) {
	current := LocationID("current")
	a := LocationID("a")
	b := LocationID("b")
	known := recoveryTestKnowledge(map[LocationID][]LocationID{
		current: {a, b},
		a:       {current},
		b:       {current},
	}, current, a, b)
	obs := Observation{Location: PlaceID(current)}
	catalog := ObjectiveCatalog{Destinations: []CatalogDestination{
		{Place: "zeta center", Location: b, Center: true, TravelCostChecked: true, TravelCostKnown: true, TravelCost: 100},
		{Place: "alpha center", Location: a, Center: true, TravelCostChecked: true, TravelCostKnown: true, TravelCost: 100},
	}}
	ranked := rankRecoveryCheckpoints(obs, known, map[LocationID]bool{current: true, a: true, b: true}, catalog, nil)
	if len(ranked) != 2 || ranked[0].Place != "alpha center" || !ranked[0].Selected {
		t.Fatalf("ranking = %+v, want alphabetical tie-break", ranked)
	}
}

func TestRecoveryProviderUsesRankedCenter(t *testing.T) {
	current := LocationID("current")
	near := LocationID("near")
	fast := LocationID("fast")
	known := recoveryTestKnowledge(map[LocationID][]LocationID{
		current: {near},
		near:    {current, fast},
		fast:    {near},
	}, current, near, fast)
	obs := Observation{
		Location:   PlaceID(current),
		PartyCount: 1,
		Party:      []PartyMon{{Species: "testmon", Level: 20, HP: 5, MaxHP: 50}},
		Catalog: ObjectiveCatalog{Destinations: []CatalogDestination{
			{Place: "near center", Location: near, Center: true, TravelCostChecked: true, TravelCostKnown: true, TravelCost: 100},
			{Place: "fast center", Location: fast, Center: true, TravelCostChecked: true, TravelCostKnown: true, TravelCost: 40, FastTravel: true, FastTravelMethod: "fly"},
		}},
	}
	offer := OfferWithEvidence(obs, known)
	if !hasCatalogObjective(offer.Candidates, Objective{Kind: KindHeal, Place: "fast center"}) {
		t.Fatalf("ranked recovery Center not offered: %+v", offer.Candidates)
	}
	if hasCatalogObjective(offer.Candidates, Objective{Kind: KindHeal, Place: "near center"}) {
		t.Fatalf("lower-ranked Center should not be offered as the deterministic recovery target: %+v", offer.Candidates)
	}
	if len(offer.Recovery) != 2 || offer.Recovery[0].Place != "fast center" || !offer.Recovery[0].Selected {
		t.Fatalf("recovery telemetry = %+v, want selected fast center first", offer.Recovery)
	}
}
