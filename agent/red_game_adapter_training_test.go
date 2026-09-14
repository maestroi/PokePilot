package agent

import (
	"errors"
	"testing"
)

func TestRedTrainingPostconditionAllowsEvolutionAtTarget(t *testing.T) {
	o := Objective{Kind: KindTrain, Species: "caterpie", Slot: 1, Level: 7}
	initial := Observation{
		Controllable: true,
		Party: []PartyMon{
			{Species: "pikachu", Level: 10},
			{Species: "caterpie", Level: 6},
		},
	}
	final := Observation{
		Controllable: true,
		Party: []PartyMon{
			{Species: "pikachu", Level: 10},
			{Species: "metapod", Level: 7},
		},
	}
	result := ObjectiveResult{Training: &TrainingEvidence{StartLevel: 6, EndLevel: 7, Reached: true}}

	if _, err := verifyObjectivePostcondition(o, initial, final, result); !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("generic verifier = %v, want species mismatch before Red-specific evolution proof", err)
	}
	adapter := &redObjectiveAdapter{}
	if err := adapter.VerifyPostcondition(o, initial, final, result); err != nil {
		t.Fatalf("Red training verifier rejected reached evolution: %v", err)
	}
}

func TestRedTrainingEvolutionFallbackRequiresReachedEvidence(t *testing.T) {
	o := Objective{Kind: KindTrain, Species: "caterpie", Slot: 1, Level: 7}
	initial := Observation{
		Controllable: true,
		Party: []PartyMon{
			{Species: "pikachu", Level: 10},
			{Species: "caterpie", Level: 6},
		},
	}
	final := Observation{
		Controllable: true,
		Party: []PartyMon{
			{Species: "pikachu", Level: 10},
			{Species: "metapod", Level: 7},
		},
	}
	result := ObjectiveResult{Training: &TrainingEvidence{StartLevel: 6, EndLevel: 7, Reached: false}}

	adapter := &redObjectiveAdapter{}
	if err := adapter.VerifyPostcondition(o, initial, final, result); !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("Red training verifier = %v, want postcondition failure without Reached evidence", err)
	}
}

func TestRedTrainingEvolutionFallbackResolvesStaleSlotHint(t *testing.T) {
	o := Objective{Kind: KindTrain, Species: "caterpie", Slot: 2, Level: 7}
	initial := Observation{
		Controllable: true,
		Party: []PartyMon{
			{Species: "pikachu", Level: 10},
			{Species: "caterpie", Level: 6},
		},
	}
	final := Observation{
		Controllable: true,
		Party: []PartyMon{
			{Species: "pikachu", Level: 10},
			{Species: "metapod", Level: 7},
		},
	}
	result := ObjectiveResult{Training: &TrainingEvidence{StartLevel: 6, EndLevel: 7, Reached: true}}

	if !redTrainingReachedThroughEvolution(o, initial, final, result) {
		t.Fatal("evolution fallback did not mirror species-based slot resolution")
	}
}
