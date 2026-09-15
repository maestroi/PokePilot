package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestRedStarterPostconditionUsesSemanticSpeciesOverride(t *testing.T) {
	obj := Objective{Kind: KindStarter, Starter: skill.StarterSquirtle, Species: "mewtwo"}
	final := Observation{
		Controllable: true,
		Party:        []PartyMon{{Species: "mewtwo", Level: 5}},
	}

	adapter := &redObjectiveAdapter{}
	if err := adapter.VerifyPostcondition(obj, Observation{}, final, ObjectiveResult{}); err != nil {
		t.Fatalf("patched starter verification failed: %v", err)
	}

	final.Party = []PartyMon{{Species: "squirtle", Level: 5}}
	if err := adapter.VerifyPostcondition(obj, Observation{}, final, ObjectiveResult{}); !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("physical middle-ball species was accepted instead of semantic mewtwo: %v", err)
	}
}

func TestRedStarterValidationRejectsUnknownSemanticSpecies(t *testing.T) {
	adapter := &redObjectiveAdapter{}
	obj := Objective{Kind: KindStarter, Starter: skill.StarterSquirtle, Species: "missingno"}
	if err := adapter.Validate(obj, Observation{}); err == nil {
		t.Fatal("unknown patched starter species was accepted")
	}
}
