package agent

import (
	"strings"
	"testing"
)

func TestFightWildEncounterPolicyClearsFleeAndDeduplicates(t *testing.T) {
	offered := []Objective{
		{Kind: KindGoTo, Place: "route 3"},
		{Kind: KindGoTo, Place: "route 3", Flee: true},
		{Kind: KindCatch, Species: "pikachu", Place: "viridian forest", Flee: true},
	}
	got := ApplyRunPolicy(Observation{}, offered, "", WildEncountersFight)
	if len(got) != 2 {
		t.Fatalf("fight policy returned %d objectives, want 2 after dedupe: %#v", len(got), got)
	}
	for _, o := range got {
		if o.Flee {
			t.Fatalf("fight policy left flee enabled: %s", o)
		}
		if strings.Contains(o.String(), "fleeing wild battles") {
			t.Fatalf("fight policy still exposes fleeing objective: %s", o)
		}
	}
}

func TestRunPolicySuppressesRepeatedSuccessfulTravelLoop(t *testing.T) {
	obs := Observation{History: []RoundRecord{
		{Objective: "go to route 22, fleeing wild battles", Outcome: "done"},
		{Objective: "go to viridian city", Outcome: "done"},
		{Objective: "go to route 22", Outcome: "done"},
		{Objective: "go to viridian city", Outcome: "done"},
	}}
	frontier := Objective{Kind: KindGoTo, Place: "route 2", Flee: true, Note: "(unvisited adjacent map)"}
	offered := []Objective{
		{Kind: KindGoTo, Place: "route 22"},
		{Kind: KindGoTo, Place: "route 22", Flee: true},
		{Kind: KindGoTo, Place: "viridian city"},
		frontier,
	}

	got := ApplyRunPolicy(obs, offered, RiskToleranceAggressive, WildEncountersPlanner)
	if len(got) != 1 || got[0].String() != frontier.String() {
		t.Fatalf("repeated travel loop policy = %#v, want only frontier %q", got, frontier.String())
	}
}

func TestRunPolicySuppressesRepeatedOptionalPurchaseButKeepsRecovery(t *testing.T) {
	buy := Objective{Kind: KindBuy, Item: ItemID("parlyz heal"), Qty: 3}
	heal := Objective{Kind: KindHeal}
	progress := Objective{Kind: KindProgress, Progress: ProgressID("deliver_oaks_parcel")}
	obs := Observation{History: []RoundRecord{
		{Objective: buy.String(), Outcome: "done"},
		{Objective: "talk at (5,5)", Outcome: "done"},
		{Objective: buy.String(), Outcome: "done"},
	}}

	got := ApplyRunPolicy(obs, []Objective{buy, heal, progress}, RiskToleranceAggressive, WildEncountersPlanner)
	if len(got) != 2 {
		t.Fatalf("optional purchase loop policy = %#v, want heal + progress", got)
	}
	for _, objective := range got {
		if objective.Kind == KindBuy {
			t.Fatalf("repeated optional purchase survived loop guard: %#v", got)
		}
	}
}

func TestRunPolicyDoesNotSuppressOneSuccessfulObjective(t *testing.T) {
	travel := Objective{Kind: KindGoTo, Place: "route 22", Flee: true}
	other := Objective{Kind: KindTalk, X: 4, Y: 4}
	obs := Observation{History: []RoundRecord{{Objective: travel.String(), Outcome: "done"}}}
	got := ApplyRunPolicy(obs, []Objective{travel, other}, RiskToleranceAggressive, WildEncountersPlanner)
	if len(got) != 2 {
		t.Fatalf("single success changed menu: %#v", got)
	}
}

func TestRunPolicyLoopGuardNeverEmptiesLegalMenu(t *testing.T) {
	route22 := Objective{Kind: KindGoTo, Place: "route 22"}
	viridian := Objective{Kind: KindGoTo, Place: "viridian city"}
	obs := Observation{History: []RoundRecord{
		{Objective: route22.String(), Outcome: "done"},
		{Objective: viridian.String(), Outcome: "done"},
		{Objective: route22.String(), Outcome: "done"},
		{Objective: viridian.String(), Outcome: "done"},
	}}
	offered := []Objective{route22, viridian}
	got := ApplyRunPolicy(obs, offered, RiskToleranceAggressive, WildEncountersPlanner)
	if len(got) != len(offered) {
		t.Fatalf("loop guard removed every legal option: %#v", got)
	}
}

func TestBalancedRiskMarksKnownCenterRecoveryBeforeTrainer(t *testing.T) {
	obs := Observation{Party: []PartyMon{{HP: 65, MaxHP: 100}}, PartyCount: 1}
	offered := []Objective{
		{Kind: KindGoTo, Place: "viridian pokemon center"},
		{Kind: KindGoTo, Place: "viridian pokemon center", Flee: true},
		{Kind: KindTrainer, X: 4, Y: 5},
	}

	legacy := ApplyRunPolicy(obs, offered, RiskToleranceAggressive, WildEncountersPlanner)
	for _, o := range legacy {
		if o.Note != "" {
			t.Fatalf("aggressive compatibility policy unexpectedly annotated menu: %#v", legacy)
		}
	}

	got := ApplyRunPolicy(obs, offered, RiskToleranceBalanced, WildEncountersPlanner)
	centers := 0
	trainerWarned := false
	for _, o := range got {
		switch {
		case o.Kind == KindGoTo && o.Place == "viridian pokemon center":
			centers++
			if !strings.Contains(o.Note, "balanced risk") {
				t.Fatalf("balanced center journey missing policy explanation: %#v", o)
			}
		case o.Kind == KindTrainer:
			trainerWarned = strings.Contains(o.Note, "recovery stop")
		case o.Kind == KindHeal:
			t.Fatalf("risk policy invented a remote heal outside raw Offer: %#v", got)
		}
	}
	if centers != 2 {
		t.Fatalf("balanced policy marked %d center journeys, want fight/flee variants: %#v", centers, got)
	}
	if !trainerWarned {
		t.Fatalf("trainer objective was not warned about available recovery: %#v", got)
	}
}

func TestCautiousRiskRecoversEarlierThanBalanced(t *testing.T) {
	obs := Observation{Party: []PartyMon{{HP: 80, MaxHP: 100}}, PartyCount: 1}
	offered := []Objective{{Kind: KindGoTo, Place: "pewter pokemon center"}}

	balanced := ApplyRunPolicy(obs, offered, RiskToleranceBalanced, WildEncountersPlanner)
	if balanced[0].Note != "" {
		t.Fatalf("balanced risk should not prefer recovery at 80%% HP: %#v", balanced)
	}

	cautious := ApplyRunPolicy(obs, offered, RiskToleranceCautious, WildEncountersPlanner)
	if !strings.Contains(cautious[0].Note, "cautious risk") {
		t.Fatalf("cautious risk should prefer the legal center journey at 80%% HP: %#v", cautious)
	}
}

func TestRunPolicyCompatibilityDefaults(t *testing.T) {
	if got := NormalizeRiskTolerance(""); got != RiskToleranceAggressive {
		t.Fatalf("empty risk tolerance = %q, want aggressive compatibility default", got)
	}
	if got := NormalizeWildEncounters(""); got != WildEncountersPlanner {
		t.Fatalf("empty wild encounter policy = %q, want planner compatibility default", got)
	}
}
