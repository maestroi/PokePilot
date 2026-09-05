package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
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
