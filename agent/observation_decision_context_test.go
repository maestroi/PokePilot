package agent_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestObservationJSONMakesOutlevelledWildBandExplicit(t *testing.T) {
	obs := agent.Observation{
		Map:        0x0c,
		Location:   "route 1",
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

	b, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("json.Unmarshal wire: %v", err)
	}
	if _, ok := wire["Map"]; ok {
		t.Fatalf("planner JSON exposes raw Red map id: %s", b)
	}

	var got struct {
		Location        agent.PlaceID
		DecisionContext *agent.DecisionContext
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got.Location != obs.Location {
		t.Fatalf("Location = %q, want %q: semantic location was dropped", got.Location, obs.Location)
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

func TestObservationJSONKeepsDexCatalogOutOfThePrompt(t *testing.T) {
	obs := agent.Observation{
		PokedexOwned: []agent.SpeciesID{"charmander"},
		Dex: agent.DexCatalog{
			Owned:   []agent.DexEntry{{Species: "charmander", Dex: 4}},
			Targets: []agent.DexEntry{{Species: "pidgey", Dex: 16, Sources: []agent.DexSource{{Kind: agent.AcquireWildGrass, Place: "route 1"}}}},
		},
	}
	b, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(b), `"wild_grass"`) {
		t.Fatalf("planner JSON embeds the full Dex catalog: %s", b)
	}
	var got struct {
		DecisionContext *agent.DecisionContext
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got.DecisionContext == nil || got.DecisionContext.Dex == nil {
		t.Fatalf("DecisionContext.Dex = %#v, want owned/target counts", got.DecisionContext)
	}
	if got.DecisionContext.Dex.Owned != 1 || got.DecisionContext.Dex.Targets != 1 {
		t.Fatalf("dex context = %+v, want owned=1 targets=1", *got.DecisionContext.Dex)
	}
}

func TestObservationJSONOmitsDecisionContextWhenIrrelevant(t *testing.T) {
	b, err := json.Marshal(agent.Observation{Map: 0x00, Location: "pallet town", MapName: "PALLET_TOWN"})
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
	if _, ok := got["Map"]; ok {
		t.Fatalf("raw Red map id present in planner observation: %s", b)
	}
}
