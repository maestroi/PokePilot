package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/red/state"
)

func TestFarmRecoveryOfferedQuarantinesRecentFailureWhenAlternativeExists(t *testing.T) {
	failed := agent.Objective{Kind: agent.KindGoTo, Place: "route 1"}
	other := agent.Objective{Kind: agent.KindGoTo, Place: "route 2"}
	obs := agent.Observation{History: []agent.RoundRecord{{
		Objective: failed.String(),
		Outcome:   "failed: skill: no path",
	}}}

	got := farmRecoveryOffered(obs, []agent.Objective{failed, other})
	if len(got) != 1 || got[0].String() != other.String() {
		t.Fatalf("offered = %v, want only %q", got, other.String())
	}
}

func TestFarmRecoveryOfferedKeepsMandatoryOnlyOption(t *testing.T) {
	failed := agent.Objective{Kind: agent.KindGoTo, Place: "mt moon b1f"}
	obs := agent.Observation{History: []agent.RoundRecord{{
		Objective: failed.String(),
		Outcome:   "failed: skill: no path",
	}}}

	got := farmRecoveryOffered(obs, []agent.Objective{failed})
	if len(got) != 1 || got[0].String() != failed.String() {
		t.Fatalf("offered = %v, want mandatory objective retained", got)
	}
}

func TestFarmRecoveryOfferedClarifiesGymChallengeWithoutHistory(t *testing.T) {
	gym := agent.Objective{Kind: agent.KindGym, Place: "pewter gym"}
	got := farmRecoveryOffered(agent.Observation{}, []agent.Objective{gym})
	if len(got) != 1 {
		t.Fatalf("offered = %v, want one gym objective", got)
	}
	if got[0].Note != farmGymChallengeNote {
		t.Fatalf("gym note = %q, want %q", got[0].Note, farmGymChallengeNote)
	}
}

func TestFarmRecoveryOfferedPrefersActiveGymOverWandering(t *testing.T) {
	gym := agent.Objective{Kind: agent.KindGym, Place: "pewter gym"}
	wander := agent.Objective{Kind: agent.KindGoTo, Place: "viridian forest"}
	train := agent.Objective{Kind: agent.KindTrain, Level: 14}

	got := farmRecoveryOffered(agent.Observation{}, []agent.Objective{wander, train, gym})
	if len(got) != 1 || got[0].Kind != agent.KindGym {
		t.Fatalf("offered = %v, want only the active gym challenge", got)
	}
	if got[0].Note != farmGymChallengeNote {
		t.Fatalf("gym note = %q, want %q", got[0].Note, farmGymChallengeNote)
	}
}

func TestFarmRecoveryOfferedKeepsGymFrontierAfterFailure(t *testing.T) {
	gym := agent.Objective{Kind: agent.KindGym, Place: "pewter gym"}
	wander := agent.Objective{Kind: agent.KindGoTo, Place: "pewter city"}
	obs := agent.Observation{History: []agent.RoundRecord{{
		Objective: gym.String(),
		Outcome:   "failed: skill: Gym: battle with BROCK did not start after the leader dialogue",
	}}}

	got := farmRecoveryOffered(obs, []agent.Objective{gym, wander})
	if len(got) != 1 || got[0].Kind != agent.KindGym {
		t.Fatalf("offered = %v, want failed progression-critical gym challenge to remain the active frontier", got)
	}
	if got[0].Note != farmGymChallengeNote {
		t.Fatalf("retained gym note = %q, want %q", got[0].Note, farmGymChallengeNote)
	}
}

func TestFarmRecoveryOfferedBoulderToCascadeSuppressesOptionalDrift(t *testing.T) {
	obs := agent.Observation{Badges: []string{state.BadgeBoulder.String()}}
	offered := []agent.Objective{
		{Kind: agent.KindCatch, Species: 1},
		{Kind: agent.KindTrain, Level: 24},
		{Kind: agent.KindGoTo, Place: "pewter gym"},
		{Kind: agent.KindGoTo, Place: "viridian city"},
		{Kind: agent.KindGoTo, Place: "pewter city"},
	}

	got := farmRecoveryOffered(obs, offered)
	if len(got) != 2 {
		t.Fatalf("offered = %v, want only non-gym travel choices while seeking Cascade", got)
	}
	for _, o := range got {
		if o.Kind != agent.KindGoTo || o.Place == "pewter gym" {
			t.Fatalf("unexpected post-Brock drift objective retained: %+v", o)
		}
	}
}

func TestFarmRecoveryOfferedBoulderToCascadeLocksOntoUnvisitedAdjacent(t *testing.T) {
	obs := agent.Observation{Badges: []string{state.BadgeBoulder.String()}}
	frontier := agent.Objective{Kind: agent.KindGoTo, Place: "route 3", Note: "(unvisited adjacent map)"}
	offered := []agent.Objective{
		{Kind: agent.KindGoTo, Place: "viridian forest"},
		{Kind: agent.KindTrain, Level: 24},
		{Kind: agent.KindCatch, Species: 1},
		frontier,
	}

	got := farmRecoveryOffered(obs, offered)
	if len(got) != 1 || got[0].String() != frontier.String() {
		t.Fatalf("offered = %v, want only forward frontier %q", got, frontier.String())
	}
}

func TestFarmRecoveryOfferedBoulderToCascadeKeepsTrainingAfterMandatoryLoss(t *testing.T) {
	obs := agent.Observation{
		Badges: []string{state.BadgeBoulder.String()},
		Failures: []agent.Failure{{
			Objective: "trainer loss while attempting go to route 3",
			Last:      "blacked out against mandatory trainer",
		}},
	}
	train := agent.Objective{Kind: agent.KindTrain, Level: 24}
	offered := []agent.Objective{
		train,
		{Kind: agent.KindCatch, Species: 1},
		{Kind: agent.KindGoTo, Place: "pewter city"},
	}

	got := farmRecoveryOffered(obs, offered)
	foundTrain := false
	for _, o := range got {
		if o.Kind == agent.KindTrain {
			foundTrain = true
		}
		if o.Kind == agent.KindCatch {
			t.Fatalf("optional catch survived first-badge bridge recovery: %v", got)
		}
	}
	if !foundTrain {
		t.Fatalf("offered = %v, want factual trainer-loss recovery to retain Train", got)
	}
}

func TestFarmRecoveryOfferedStopsFirstBadgeSteeringAfterCascade(t *testing.T) {
	obs := agent.Observation{Badges: []string{
		state.BadgeBoulder.String(),
		state.BadgeCascade.String(),
	}}
	offered := []agent.Objective{
		{Kind: agent.KindCatch, Species: 1},
		{Kind: agent.KindTrain, Level: 24},
		{Kind: agent.KindGoTo, Place: "pewter gym"},
	}

	got := farmRecoveryOffered(obs, offered)
	if len(got) != len(offered) {
		t.Fatalf("offered = %v, want first-badge steering disabled after Cascade", got)
	}
}

func TestFarmRecoveryOfferedExpiresAfterTwoRounds(t *testing.T) {
	failed := agent.Objective{Kind: agent.KindGoTo, Place: "route 1"}
	other := agent.Objective{Kind: agent.KindGoTo, Place: "route 2"}
	obs := agent.Observation{History: []agent.RoundRecord{
		{Objective: failed.String(), Outcome: "failed: skill: no path"},
		{Objective: other.String(), Outcome: "done"},
		{Objective: other.String(), Outcome: "done"},
	}}

	got := farmRecoveryOffered(obs, []agent.Objective{failed, other})
	if len(got) != 2 {
		t.Fatalf("offered = %v, want cooldown expired", got)
	}
}
