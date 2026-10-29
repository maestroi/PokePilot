package agent

import (
	"strings"
	"testing"
)

func offeredBuy(objs []Objective, item ItemID) (Objective, bool) {
	for _, o := range objs {
		if o.Kind == KindBuy && o.Item == item {
			return o, true
		}
	}
	return Objective{}, false
}

func TestOfferMartBuyQuantityIsActuallyAffordable(t *testing.T) {
	pokeball, ok := ItemByName("pokeball")
	if !ok {
		t.Fatal("pokeball missing from item table")
	}
	obs := Observation{
		MapName:   "viridian mart",
		Money:     375,
		MartStock: []string{"pokeball"},
	}
	buy, ok := offeredBuy(Offer(obs, NewKnowledge(nil)), pokeball)
	if !ok {
		t.Fatal("POKEBALL not offered with enough money for one")
	}
	if buy.Qty != 1 {
		t.Fatalf("POKEBALL quantity = %d, want 1 with 375 money", buy.Qty)
	}

	obs.Money = 199
	if buy, ok := offeredBuy(Offer(obs, NewKnowledge(nil)), pokeball); ok {
		t.Fatalf("unaffordable POKEBALL was offered: %+v", buy)
	}
}

func TestOfferMartRespectsEconomyReserve(t *testing.T) {
	pokeball, ok := ItemByName("pokeball")
	if !ok {
		t.Fatal("pokeball missing from item table")
	}
	obs := Observation{
		MapName:   "viridian mart",
		Money:     1600,
		Bag:       []Item{{Name: "poke flute", Quantity: 1}},
		MartStock: []string{"pokeball"},
	}
	if buy, ok := offeredBuy(Offer(obs, NewKnowledge(nil)), pokeball); ok {
		t.Fatalf("purchase consumed reserved Fuchsia money: %+v", buy)
	}
}

func TestRestockCaptureObjectivesOffersReachableDexBallSupply(t *testing.T) {
	obs := Observation{
		Money:        2000,
		RestockStock: []string{"pokeball", "potion"},
		Dex:          DexCatalog{Targets: []DexEntry{{Species: "rattata"}}},
	}
	got := restockCaptureObjectives(obs)
	if len(got) != 1 {
		t.Fatalf("capture restock = %+v, want one remote buy", got)
	}
	buy := got[0]
	if buy.Kind != KindBuy || buy.Item != "pokeball" || buy.Qty != targetCaptureStock || buy.Intent != dexCaptureSupplyIntent {
		t.Fatalf("capture restock = %+v, want %d POKEBALL travel-and-buy", buy, targetCaptureStock)
	}

	obs.Bag = []Item{{Name: "pokeball", Quantity: minimumCaptureStock}}
	if got := restockCaptureObjectives(obs); len(got) != 0 {
		t.Fatalf("capture stock at minimum still offered remote restock: %+v", got)
	}

	obs.Bag = nil
	obs.Dex.Targets = nil
	if got := restockCaptureObjectives(obs); len(got) != 0 {
		t.Fatalf("completed Dex still offered remote restock: %+v", got)
	}
}

// run-d6dokr184ky81: ¥229 below the ¥500 Safari entry, Fuchsia still owed
// Surf/Strength, and nothing offered could raise money for 73 attempts.
func TestFundingSaleObjectivesCoverUnaffordableStoryPayment(t *testing.T) {
	obs := Observation{
		Money:        229,
		RestockStock: []string{"pokeball"},
		Bag: []Item{
			{Name: "poke flute", Quantity: 1},
			{Name: "hm05", Quantity: 1},
			{Name: "moon stone", Quantity: 1},
			{Name: "ultra ball", Quantity: 9},
			{Name: "nugget", Quantity: 1},
		},
	}
	got := map[string]int{}
	for _, o := range fundingSaleObjectives(obs) {
		if o.Kind != KindSell || o.Validate() != nil {
			t.Fatalf("funding offer %+v is not a valid sale", o)
		}
		if _, ok := redItemID(o.Item); !ok {
			t.Fatalf("funding offer %+v does not resolve to a Red item", o)
		}
		got[string(o.Item)] = o.Qty
	}
	// ¥271 short: one ULTRA BALL (¥600) or the NUGGET (¥5000) covers it.
	// Key items and the zero-price MOON STONE are never offered.
	if len(got) != 2 || got["ultra ball"] != 1 || got["nugget"] != 1 {
		t.Fatalf("funding sales = %v, want 1 ULTRA BALL and 1 NUGGET", got)
	}

	obs.Money = 500
	if got := fundingSaleObjectives(obs); len(got) != 0 {
		t.Fatalf("funded checkpoint still offered sales: %+v", got)
	}
	obs.Money = 229
	obs.RestockStock = nil
	if got := fundingSaleObjectives(obs); len(got) != 0 {
		t.Fatalf("sales offered with no reachable mart: %+v", got)
	}
}

// run-d6dokr184ky81 (later): post-Champion Dex run at 79/139, ¥87, only a
// MASTER BALL, and nothing offered but town-to-town travel.
func TestFundingSaleObjectivesCoverDryDexCaptureStock(t *testing.T) {
	obs := Observation{
		Money:        87,
		RestockStock: []string{"pokeball"},
		Dex:          DexCatalog{Targets: []DexEntry{{Species: "rattata"}}},
		Bag: []Item{
			{Name: "master ball", Quantity: 1},
			{Name: "max revive", Quantity: 1},
			{Name: "guard spec", Quantity: 1},
		},
	}
	got := map[string]int{}
	for _, o := range fundingSaleObjectives(obs) {
		got[string(o.Item)] = o.Qty
	}
	// ¥913 short of five POKE BALLs: the MAX REVIVE (¥2000) covers it; one
	// GUARD SPEC (¥350) does not.
	if len(got) != 1 || got["max revive"] != 1 {
		t.Fatalf("funding sales = %v, want 1 MAX REVIVE", got)
	}
	obs.Bag = append(obs.Bag, Item{Name: "pokeball", Quantity: minimumCaptureStock})
	if got := fundingSaleObjectives(obs); len(got) != 0 {
		t.Fatalf("stocked Dex run still offered sales: %+v", got)
	}
}

// run-d6dokr184ky81 (88/139): ¥192, only a MASTER BALL, and a bag whose
// sellable stock was two TMs plus one GUARD SPEC. TMs had no price, and the
// GUARD SPEC alone could not cover the shortfall, so nothing was offered.
func TestFundingSaleObjectivesSellTMsAndFallBackToPartialSales(t *testing.T) {
	obs := Observation{
		Money:        192,
		RestockStock: []string{"pokeball"},
		Dex:          DexCatalog{Targets: []DexEntry{{Species: "rattata"}}},
		Bag: []Item{
			{Name: "master ball", Quantity: 1},
			{Name: "guard spec", Quantity: 1},
			{Name: "moon stone", Quantity: 1},
			{Name: "tm46", Quantity: 1},
			{Name: "tm47", Quantity: 1},
		},
	}
	sales := func() map[string]int {
		got := map[string]int{}
		for _, o := range fundingSaleObjectives(obs) {
			if o.Kind != KindSell || o.Validate() != nil {
				t.Fatalf("funding offer %+v is not a valid sale", o)
			}
			if _, ok := redItemID(o.Item); !ok {
				t.Fatalf("funding offer %+v does not resolve to a Red item", o)
			}
			got[string(o.Item)] = o.Qty
		}
		return got
	}
	// ¥808 short: TM46 (¥2000) and TM47 (¥1500) each cover it.
	if got := sales(); len(got) != 2 || got["tm46"] != 1 || got["tm47"] != 1 {
		t.Fatalf("funding sales = %v, want TM46 and TM47", got)
	}
	// Without the TMs nothing covers it; the GUARD SPEC still raises ¥350.
	obs.Bag = obs.Bag[:3]
	if got := sales(); len(got) != 1 || got["guard spec"] != 1 {
		t.Fatalf("funding sales = %v, want the partial GUARD SPEC sale", got)
	}
}

// run-d6dokr184ky81 (93/139): ¥42, only a MASTER BALL, nothing sellable, and
// a party grown to level 100 shuttling between towns, because trainers were
// only ever offered on the map the player already stood on. A known route
// with unbeaten trainers must surface as a funding destination.
func TestTravelOffersUnbeatenTrainerRouteWhenDexRunIsBroke(t *testing.T) {
	obs := Observation{
		GameID:     testGameID,
		Map:        1,
		Location:   "town",
		PartyCount: 1,
		Party:      []PartyMon{{Species: "testmon", Level: 100, HP: 300, MaxHP: 300}},
		Money:      42,
		Dex:        DexCatalog{Targets: []DexEntry{{Species: "rattata"}}},
		Bag:        []Item{{Name: "master ball", Quantity: 1}},
		Catalog: ObjectiveCatalog{Destinations: []CatalogDestination{
			{Place: "quiet town", Location: "quiet"},
			{Place: "trainer route", Location: "route", TrainerPrize: 1200},
		}},
	}
	known := NewKnowledge(&KnowledgeTopology{Adjacency: map[LocationID][]LocationID{
		"town": {"quiet", "route"}, "quiet": {"town"}, "route": {"town"},
	}})
	for _, location := range []LocationID{"town", "quiet", "route"} {
		known.SawLocation(location)
	}
	funded := func() (Objective, bool) {
		for _, o := range OfferWithEvidence(obs, known).Candidates {
			if o.Kind == KindGoTo && o.Place == "trainer route" && strings.Contains(o.Note, "funding") {
				return o, true
			}
		}
		return Objective{}, false
	}
	if o, ok := funded(); !ok || !strings.Contains(o.Note, "¥1200") {
		t.Fatalf("broke Dex run offered no funding trainer route: %+v", OfferWithEvidence(obs, known).Candidates)
	}
	obs.Bag = append(obs.Bag, Item{Name: "pokeball", Quantity: minimumCaptureStock})
	if o, ok := funded(); ok {
		t.Fatalf("stocked Dex run still tagged a funding route: %s %s", o, o.Note)
	}
}
