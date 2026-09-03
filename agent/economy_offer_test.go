package agent

import "testing"

func offeredBuy(objs []Objective, item uint8) (Objective, bool) {
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
