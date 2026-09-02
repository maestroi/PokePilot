package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestObservationJSONMakesOutlevelledWildBandExplicit(t *testing.T) {
	obs := agent.Observation{
		Map:        0x0c,
		MapName:    "ROUTE_1",
		HasGrass:   true,
		PartyCount: 1,
		Party: []agent.PartyMon{
			{Level: 22, HP: 60, MaxHP: 60},
		},
		WildGrass: []agent.WildSpecies{
			{Name: "pidgey", MinLevel: 2, MaxLevel: 5, Slots: 6},
			{Name: "rattata", MinLevel: 2, MaxLevel: 4, Slots: 4},
		},
	}

	var got struct {
		Map             uint8
		DecisionContext *agent.DecisionContext
	}
	b, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got.Map != obs.Map {
		t.Fatalf("Map = %#02x, want %#02x: MarshalJSON dropped an existing Observation field", got.Map, obs.Map)
	}
	if got.DecisionContext == nil || got.DecisionContext.Training == nil {
		t.Fatalf("DecisionContext.Training = %#v, want local level comparison", got.DecisionContext)
	}
	ctx := got.DecisionContext.Training
	if ctx.LeadLevel != 22 || ctx.WildMinLevel != 2 || ctx.WildMaxLevel != 5 || ctx.LeadLevelMinusWildMax != 17 {
		t.Fatalf("training context = %+v, want lead 22 vs wilds 2-5 (gap +17)", *ctx)
	}
}

func TestObservationJSONStatesCatchDoesNotDamageWantedTarget(t *testing.T) {
	obs := agent.Observation{
		Bag: []agent.Item{{Name: "pokeball", Quantity: 5}},
		WildGrass: []agent.WildSpecies{
			{Name: "pidgey", MinLevel: 2, MaxLevel: 5, Slots: 6},
		},
	}

	var got struct {
		DecisionContext *agent.DecisionContext
	}
	b, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got.DecisionContext == nil || got.DecisionContext.Catch == nil {
		t.Fatalf("DecisionContext.Catch = %#v, want catch semantics", got.DecisionContext)
	}
	ctx := got.DecisionContext.Catch
	if ctx.WantedTargetsAttacked || ctx.WantedTargetsWeakened || !ctx.BallsThrownAtFullHP {
		t.Fatalf("catch context = %+v, want no attack, no weakening, full-HP throws", *ctx)
	}
}

func TestObservationJSONOmitsDecisionContextWhenIrrelevant(t *testing.T) {
	b, err := json.Marshal(agent.Observation{Map: 0x00, MapName: "PALLET_TOWN"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := got["DecisionContext"]; ok {
		t.Fatalf("DecisionContext present in irrelevant observation: %s", b)
	}
}
