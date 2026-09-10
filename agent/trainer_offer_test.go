package agent_test

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/skill"
)

func hasObjective(offered []agent.Objective, want agent.Objective) bool {
	for _, got := range offered {
		if got == want {
			return true
		}
	}
	return false
}

func TestOfferChallengeableTrainer(t *testing.T) {
	obs := agent.Observation{
		Map:        0x0e,
		PartyCount: 1,
		MapObjects: []agent.MapObject{
			{X: 10, Y: 6, Kind: "trainer", Challengeable: true},
			{X: 14, Y: 4, Kind: "trainer", Challengeable: true, Defeated: true},
			{X: 5, Y: 5, Kind: "trainer"}, // story/custom: not generic
		},
	}
	known := agent.NewKnowledge(nil)
	want := agent.Objective{Kind: agent.KindTrainer, X: 10, Y: 6}
	if offered := agent.Offer(obs, known); !hasObjective(offered, want) {
		t.Fatalf("Offer omitted live undefeated trainer %q: %v", want, offered)
	}
	for _, bad := range []agent.Objective{
		{Kind: agent.KindTrainer, X: 14, Y: 4},
		{Kind: agent.KindTrainer, X: 5, Y: 5},
	} {
		if offered := agent.Offer(obs, known); hasObjective(offered, bad) {
			t.Fatalf("Offer exposed defeated/story trainer %q: %v", bad, offered)
		}
	}
}

func TestCompletedTrainerIsNotReoffered(t *testing.T) {
	obs := agent.Observation{
		Map:        0x0e,
		PartyCount: 1,
		MapObjects: []agent.MapObject{{X: 10, Y: 6, Kind: "trainer", Challengeable: true}},
	}
	known := agent.NewKnowledge(nil)
	challenge := agent.Objective{Kind: agent.KindTrainer, X: 10, Y: 6}
	known.Done(challenge)
	if offered := agent.Offer(obs, known); hasObjective(offered, challenge) {
		t.Fatalf("completed trainer was re-offered: %v", offered)
	}
}

func TestTrainerLossUsesExistingRecoveryGate(t *testing.T) {
	obs := agent.Observation{
		Map:        0x0e,
		PartyCount: 1,
		MapObjects: []agent.MapObject{{X: 10, Y: 6, Kind: "trainer", Challengeable: true}},
	}
	known := agent.NewKnowledge(nil)
	challenge := agent.Objective{Kind: agent.KindTrainer, X: 10, Y: 6}
	known.Failed(challenge, errors.New("wrapped: "+skill.ErrTrainerBlackedOut.Error()))
	// errors.New above is intentionally not wrapping; prove the typed path too.
	known = agent.NewKnowledge(nil)
	known.Failed(challenge, errors.Join(errors.New("battle failed"), skill.ErrTrainerBlackedOut))
	if offered := agent.Offer(obs, known); hasObjective(offered, challenge) {
		t.Fatalf("trainer loss did not gate immediate rechallenge: %v", offered)
	}
}

func TestTrainerObjectiveString(t *testing.T) {
	o := agent.Objective{Kind: agent.KindTrainer, X: 10, Y: 6}
	if got, want := o.String(), "challenge trainer at (10,6)"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if err := o.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}
