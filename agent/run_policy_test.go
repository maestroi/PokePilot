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

func TestBalancedRiskAddsKnownCenterRecoveryBeforeTrainer(t *testing.T) {
	obs := Observation{Party: []PartyMon{{HP: 65, MaxHP: 100}}, PartyCount: 1}
	offered := []Objective{
		{Kind: KindGoTo, Place: "viridian pokemon center"},
		{Kind: KindGoTo, Place: "viridian pokemon center", Flee: true},
		{Kind: KindTrainer, X: 4, Y: 5},
	}

	legacy := ApplyRunPolicy(obs, offered, RiskToleranceAggressive, WildEncountersPlanner)
	for _, o := range legacy {
		if o.Kind == KindHeal {
			t.Fatalf("aggressive compatibility policy unexpectedly synthesized heal: %#v", legacy)
		}
	}

	got := ApplyRunPolicy(obs, offered, RiskToleranceBalanced, WildEncountersPlanner)
	heals := 0
	trainerWarned := false
	for _, o := range got {
		switch o.Kind {
		case KindHeal:
			heals++
			if !strings.Contains(o.Note, "balanced risk") {
				t.Fatalf("balanced heal missing policy explanation: %#v", o)
			}
		case KindTrainer:
			trainerWarned = strings.Contains(o.Note, "recovery stop")
		}
	}
	if heals != 2 {
		t.Fatalf("balanced policy synthesized %d heals, want fight/flee center variants: %#v", heals, got)
	}
	if !trainerWarned {
		t.Fatalf("trainer objective was not warned about available recovery: %#v", got)
	}
}

func TestCautiousRiskRecoversEarlierThanBalanced(t *testing.T) {
	obs := Observation{Party: []PartyMon{{HP: 80, MaxHP: 100}}, PartyCount: 1}
	offered := []Objective{{Kind: KindGoTo, Place: "pewter pokemon center"}}

	balanced := ApplyRunPolicy(obs, offered, RiskToleranceBalanced, WildEncountersPlanner)
	for _, o := range balanced {
		if o.Kind == KindHeal {
			t.Fatalf("balanced risk should not recover at 80%% HP: %#v", balanced)
		}
	}

	cautious := ApplyRunPolicy(obs, offered, RiskToleranceCautious, WildEncountersPlanner)
	if len(cautious) < 2 || cautious[0].Kind != KindHeal {
		t.Fatalf("cautious risk should add an early recovery at 80%% HP: %#v", cautious)
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
