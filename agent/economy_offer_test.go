package agent

import "testing"

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
