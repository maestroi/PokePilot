package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestValidateIntentFactsRejectsUnobservedBrockVictory(t *testing.T) {
	obs := Observation{}
	err := validateIntentFacts("Get to Cerulean City (Brock defeated, Mt. Moon next)", obs)
	if !errors.Is(err, ErrIntentContradictsObservation) {
		t.Fatalf("validateIntentFacts error = %v, want ErrIntentContradictsObservation", err)
	}
}

func TestValidateIntentFactsAllowsFutureBrockGoal(t *testing.T) {
	if err := validateIntentFacts("Defeat Brock and earn the Boulder Badge", Observation{}); err != nil {
		t.Fatalf("future-facing intent was rejected: %v", err)
	}
}

func TestValidateIntentFactsAllowsObservedBrockVictory(t *testing.T) {
	obs := Observation{Badges: []string{state.BadgeBoulder.String()}}
	if err := validateIntentFacts("Get to Cerulean City (Brock defeated, Mt. Moon next)", obs); err != nil {
		t.Fatalf("observed Brock victory was rejected: %v", err)
	}
}
