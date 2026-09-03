package agent

import "testing"

func TestEconomyContextReservesKnownSafariSpend(t *testing.T) {
	obs := Observation{
		Money: 4000,
		Bag:   []Item{{Name: "poke flute", Quantity: 1}},
	}
	ctx := EconomyContext(obs)
	if ctx == nil {
		t.Fatal("EconomyContext = nil, want Fuchsia reserve")
	}
	if ctx.ReservedMoney != 1500 || ctx.SpendableMoney != 2500 {
		t.Fatalf("reserve/spendable = %d/%d, want 1500/2500", ctx.ReservedMoney, ctx.SpendableMoney)
	}
	if ctx.MoneyAfterBlackout != 2000 || ctx.BlackoutLoss != 2000 || !ctx.ReserveSurvivesBlackout {
		t.Fatalf("blackout context = after %d loss %d survives %v, want 2000/2000/true",
			ctx.MoneyAfterBlackout, ctx.BlackoutLoss, ctx.ReserveSurvivesBlackout)
	}

	complete := obs
	complete.Badges = []string{"Soul"}
	complete.Bag = append(complete.Bag,
		Item{Name: "hm03", Quantity: 1},
		Item{Name: "hm04", Quantity: 1},
	)
	completeCtx := EconomyContext(complete)
	if completeCtx == nil {
		t.Fatal("EconomyContext(complete) = nil, want bag diagnostics")
	}
	if completeCtx.ReservedMoney != 0 {
		t.Fatalf("completed Fuchsia reserve = %d, want 0", completeCtx.ReservedMoney)
	}
}

func TestEconomyCanAffordButShouldNotSpendReservedMoney(t *testing.T) {
	obs := Observation{
		Money:     1600,
		Bag:       []Item{{Name: "poke flute", Quantity: 1}},
		Party:     []PartyMon{{HP: 10, MaxHP: 60}},
		PartyCount: 1,
		MartStock: []string{"super potion"},
	}
	ctx := EconomyContext(obs)
	p := purchase(t, ctx, "super potion")
	if !p.CanAfford {
		t.Fatal("CanAfford = false, want true from total cash")
	}
	if p.CanAffordAfterReserve {
		t.Fatal("CanAffordAfterReserve = true, want false")
	}
	if p.ShouldBuy || p.SuggestedQty != 0 {
		t.Fatalf("ShouldBuy/qty = %v/%d, want false/0 while progression money is reserved", p.ShouldBuy, p.SuggestedQty)
	}
}

func TestEconomyBoundedBallResupply(t *testing.T) {
	obs := Observation{
		Money: 5000,
		Bag:   []Item{{Name: "pokeball", Quantity: 2}},
		WildGrass: []WildSpecies{{Name: "pidgey", MinLevel: 2, MaxLevel: 5, Slots: 10}},
		MartStock: []string{"pokeball", "great ball"},
	}
	ctx := EconomyContext(obs)
	if !ctx.ResupplyNeeded {
		t.Fatal("ResupplyNeeded = false with 2 balls and catchable grass")
	}
	great := purchase(t, ctx, "great ball")
	if !great.ShouldBuy || great.SuggestedQty != 8 || great.TargetStock != 10 || great.CategoryStock != 2 {
		t.Fatalf("great-ball advice = %+v, want bounded resupply of 8 toward 10 total", great)
	}
	poke := purchase(t, ctx, "pokeball")
	if poke.ShouldBuy {
		t.Fatalf("pokeball ShouldBuy = true while stronger affordable Great Balls are stocked: %+v", poke)
	}
}

func TestEconomyHealingPrefersFreeCenterAndBoundsEmergencyStock(t *testing.T) {
	obs := Observation{
		Money:      3000,
		PartyCount: 1,
		Party:      []PartyMon{{HP: 10, MaxHP: 60}},
		MartStock:  []string{"potion", "super potion"},
	}
	ctx := EconomyContext(obs)
	if !ctx.PreferFreeCenterHealing {
		t.Fatal("PreferFreeCenterHealing = false for hurt party")
	}
	super := purchase(t, ctx, "super potion")
	if !super.ShouldBuy || super.SuggestedQty != 2 || super.TargetStock != 2 {
		t.Fatalf("super-potion advice = %+v, want bounded emergency stock of 2", super)
	}
	potion := purchase(t, ctx, "potion")
	if potion.ShouldBuy {
		t.Fatalf("potion ShouldBuy = true, want the HP-appropriate stocked medicine instead: %+v", potion)
	}
}

func TestEconomyDoesNotSpeculateOnStonesOrTravelUtility(t *testing.T) {
	obs := Observation{
		Money:     9999,
		MartStock: []string{"fire stone", "repel"},
	}
	ctx := EconomyContext(obs)
	for _, name := range []string{"fire stone", "repel"} {
		p := purchase(t, ctx, name)
		if !p.CanAffordAfterReserve {
			t.Fatalf("%s unexpectedly unaffordable: %+v", name, p)
		}
		if p.ShouldBuy {
			t.Fatalf("%s ShouldBuy = true without a concrete need: %+v", name, p)
		}
	}
}

func TestEconomyBlackoutRiskCanConsumeFutureReserve(t *testing.T) {
	obs := Observation{
		Money: 2999,
		Bag:   []Item{{Name: "poke flute", Quantity: 1}},
	}
	ctx := EconomyContext(obs)
	if ctx.MoneyAfterBlackout != 1499 || ctx.BlackoutLoss != 1500 {
		t.Fatalf("blackout money/loss = %d/%d, want 1499/1500", ctx.MoneyAfterBlackout, ctx.BlackoutLoss)
	}
	if ctx.ReserveSurvivesBlackout {
		t.Fatal("ReserveSurvivesBlackout = true, want false: a blackout would put the ¥1500 Safari reserve out of reach")
	}
	if ctx.SpendableAfterBlackout != 0 {
		t.Fatalf("SpendableAfterBlackout = %d, want 0", ctx.SpendableAfterBlackout)
	}
}

func TestEconomyBagCapacityBlocksNewItemType(t *testing.T) {
	bag := make([]Item, 20)
	for i := range bag {
		bag[i] = Item{Name: "occupied slot", Quantity: 1}
	}
	obs := Observation{Money: 5000, Bag: bag, MartStock: []string{"great ball"}}
	ctx := EconomyContext(obs)
	if ctx.BagSlotsFree != 0 {
		t.Fatalf("BagSlotsFree = %d, want 0", ctx.BagSlotsFree)
	}
	p := purchase(t, ctx, "great ball")
	if p.CanStore || p.ShouldBuy {
		t.Fatalf("full-bag advice = %+v, want CanStore=false ShouldBuy=false", p)
	}
}

func TestEconomyExtendsLaterMartItemVocabulary(t *testing.T) {
	cases := map[string]uint8{
		"ultra ball": 0x02,
		"fire stone": 0x20,
		"leaf stone": 0x2F,
		"full heal":  0x34,
		"revive":     0x35,
		"max repel":  0x39,
	}
	for name, want := range cases {
		id, ok := ItemByName(name)
		if !ok || id != want {
			t.Errorf("ItemByName(%q) = %#02x,%v, want %#02x,true", name, id, ok, want)
			continue
		}
		gotName, ok := ItemName(want)
		if !ok || gotName != name {
			t.Errorf("ItemName(%#02x) = %q,%v, want %q,true", want, gotName, ok, name)
		}
	}
}

func purchase(t *testing.T, ctx *EconomyDecisionContext, name string) PurchaseAdvice {
	t.Helper()
	if ctx == nil {
		t.Fatalf("EconomyContext = nil looking for %q", name)
	}
	for _, p := range ctx.Purchases {
		if p.Item == name {
			return p
		}
	}
	t.Fatalf("purchase advice for %q not found in %+v", name, ctx.Purchases)
	return PurchaseAdvice{}
}
