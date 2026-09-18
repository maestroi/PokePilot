package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/game"
)

func TestFailureTelemetryUsesObservationGameIdentity(t *testing.T) {
	result := agent.ObjectiveResult{
		Objective: agent.Objective{Kind: agent.KindGoTo, Place: "viridian city"},
		Outcome:   agent.OutcomeBlocked,
		Cause:     "test",
		Final: agent.Observation{
			GameID: game.GameID("pokemon-yellow"),
		},
	}
	got := farmIdentityFromAgent(result)
	if got.Adapter != "pokemon-yellow" {
		t.Fatalf("adapter = %q, want pokemon-yellow", got.Adapter)
	}
}
