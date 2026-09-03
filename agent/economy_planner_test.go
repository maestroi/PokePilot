package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlannerEconomySuppressesRepeatedFailedPurchase(t *testing.T) {
	obs := Observation{
		Money:     5000,
		Bag:       []Item{{Name: "pokeball", Quantity: 2}},
		WildGrass: []WildSpecies{{Name: "pidgey", MinLevel: 2, MaxLevel: 5, Slots: 10}},
		MartStock: []string{"great ball"},
		Failures: []Failure{{
			Objective: "buy 3 GREAT BALL",
			Times:     2,
			Last:      "shop interaction failed",
		}},
	}
	ctx := plannerEconomyContext(obs)
	p := purchase(t, ctx, "great ball")
	if p.ShouldBuy || p.SuggestedQty != 0 || p.SuggestedCost != 0 {
		t.Fatalf("repeated-failure advice = %+v, want purchase suppressed", p)
	}
	if !strings.Contains(p.Reason, "failed 2x") {
		t.Fatalf("failure rationale = %q, want visible repeated-failure count", p.Reason)
	}
}

func TestObservationJSONIncludesEconomyRationale(t *testing.T) {
	obs := Observation{
		Money:      3000,
		PartyCount: 1,
		Party:      []PartyMon{{HP: 10, MaxHP: 60}},
		MartStock:  []string{"potion", "super potion"},
	}
	encoded, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		DecisionContext *DecisionContext
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.DecisionContext == nil || got.DecisionContext.Economy == nil {
		t.Fatalf("planner JSON has no economy context: %s", encoded)
	}
	p := purchase(t, got.DecisionContext.Economy, "super potion")
	if p.Reason == "" || p.SuggestedQty != 2 {
		t.Fatalf("serialized purchase rationale = %+v, want bounded medicine advice", p)
	}
}

func TestEconomyCatalogClassifiesStrategicInventory(t *testing.T) {
	cases := map[string]InventoryCategory{
		"master ball": InventoryCapture,
		"moon stone":  InventoryEvolution,
		"hm01":        InventoryProgressionCritical,
		"soda pop":    InventoryBattleConsumable,
		"max repel":   InventoryTravel,
		"poke doll":   InventoryOptionalUtility,
	}
	for name, want := range cases {
		spec, ok := ItemEconomy(name)
		if !ok || spec.Category != want {
			t.Errorf("ItemEconomy(%q) = %+v,%v, want category %q", name, spec, ok, want)
		}
	}
}

func TestLowBallStockSignalsResupplyAlongsideCatchContext(t *testing.T) {
	obs := Observation{
		Money:     2000,
		HasGrass:  true,
		Bag:       []Item{{Name: "pokeball", Quantity: 2}},
		WildGrass: []WildSpecies{{Name: "pidgey", MinLevel: 2, MaxLevel: 5, Slots: 10}},
	}
	ctx := decisionContextFor(obs)
	if ctx == nil || ctx.Catch == nil || ctx.Economy == nil {
		t.Fatalf("decision context = %+v, want catch and economy context together", ctx)
	}
	if !ctx.Economy.ResupplyNeeded || !strings.Contains(ctx.Economy.ResupplyReason, "capture stock is low") {
		t.Fatalf("resupply = %v %q, want low-ball signal before more catch attempts",
			ctx.Economy.ResupplyNeeded, ctx.Economy.ResupplyReason)
	}
}

func TestBossFailureCanTriggerRecoveryResupply(t *testing.T) {
	obs := Observation{
		Money: 3000,
		Failures: []Failure{{
			Objective: "beat the gym leader here",
			Times:     1,
			Last:      "blacked out",
		}},
	}
	ctx := plannerEconomyContext(obs)
	if ctx == nil || !ctx.ResupplyNeeded || !strings.Contains(ctx.ResupplyReason, "boss objective failed") {
		t.Fatalf("boss recovery context = %+v, want bounded recovery resupply signal", ctx)
	}
}
