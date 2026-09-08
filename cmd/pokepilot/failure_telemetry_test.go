package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func structuredFailureResult(cause agent.FailureCauseID, level uint8) agent.ObjectiveResult {
	initial := agent.FailureState{
		Location:     "mt moon b1f",
		X:            10,
		Y:            22,
		Controllable: true,
		Party: []agent.FailurePartyMember{{
			Species: "pikachu",
			Level:   level,
			HP:      20,
			MaxHP:   20,
		}},
	}
	return agent.ObjectiveResult{
		Objective: agent.Objective{
			Kind:  agent.KindGoTo,
			Place: "mt moon b1f",
			Flee:  true,
		},
		Outcome: agent.OutcomeBlocked,
		Summary: "blocked at a stable route boundary",
		Cause:   cause,
		Initial: &initial,
		Final: agent.Observation{
			Map:          0x3b,
			Location:     "mt moon b1f",
			X:            10,
			Y:            22,
			Controllable: true,
			Party: []agent.PartyMon{{
				Species: "pikachu",
				Level:   level,
				HP:      20,
				MaxHP:   20,
			}},
		},
	}
}

func TestObjectiveFailureTelemetryMarksRepeatedUnrecoveredFailureBlocking(t *testing.T) {
	resetObjectiveFailureTelemetry()
	t.Cleanup(resetObjectiveFailureTelemetry)

	failure := structuredFailureResult("blocked_step", 10)
	captureObjectiveFailureTelemetry(agent.Result{Outcomes: []agent.ObjectiveResult{failure, failure}})

	got, terminal := drainObjectiveFailureTelemetry("failed", "build-a", "")
	if len(got) != 1 {
		t.Fatalf("failures = %+v, want one group", got)
	}
	f := got[0]
	if f.Count != 2 || f.FirstRound != 1 || f.LastRound != 2 {
		t.Fatalf("round/count summary = %+v", f)
	}
	if f.Map != 0x3b || f.X != 10 || f.Y != 22 {
		t.Fatalf("location = %+v", f)
	}
	if f.Recovered || !f.Blocking {
		t.Fatalf("classification = recovered=%t blocking=%t, want false/true", f.Recovered, f.Blocking)
	}
	if f.ObservedAt.IsZero() || f.Fingerprint == "" || f.Key == "" || f.Identity == nil {
		t.Fatalf("structured identity missing from %+v", f)
	}
	if terminal == nil || terminal.Fingerprint != f.Fingerprint {
		t.Fatalf("terminal occurrence = %+v, want fingerprint %q", terminal, f.Fingerprint)
	}
}

func TestObjectiveFailureTelemetryMarksLaterProgressRecovered(t *testing.T) {
	resetObjectiveFailureTelemetry()
	t.Cleanup(resetObjectiveFailureTelemetry)

	failure := structuredFailureResult("no_route", 10)
	progressed := agent.ObjectiveResult{
		Objective: agent.Objective{Kind: agent.KindTrain, Level: 11},
		Outcome:   agent.OutcomeCompleted,
		Final: agent.Observation{
			Location:     "mt moon b1f",
			X:            10,
			Y:            22,
			Controllable: true,
			Party: []agent.PartyMon{{
				Species: "pikachu",
				Level:   11,
				HP:      20,
				MaxHP:   20,
			}},
		},
	}
	captureObjectiveFailureTelemetry(agent.Result{Outcomes: []agent.ObjectiveResult{failure, progressed}})

	got, _ := drainObjectiveFailureTelemetry("budget", "build-a", "")
	if len(got) != 1 {
		t.Fatalf("failures = %+v, want one group", got)
	}
	if !got[0].Recovered || got[0].Blocking {
		t.Fatalf("classification = recovered=%t blocking=%t, want true/false", got[0].Recovered, got[0].Blocking)
	}
}

func TestObjectiveFailureTelemetryExcludesExpectedGameOutcome(t *testing.T) {
	resetObjectiveFailureTelemetry()
	t.Cleanup(resetObjectiveFailureTelemetry)

	failure := structuredFailureResult("blacked_out", 10)
	captureObjectiveFailureTelemetry(agent.Result{Outcomes: []agent.ObjectiveResult{failure}})
	got, terminal := drainObjectiveFailureTelemetry("budget", "build-a", "")
	if len(got) != 0 || terminal != nil {
		t.Fatalf("blackout produced engineering failure telemetry: %+v terminal=%+v", got, terminal)
	}
}

func TestAgentLogProseIsNotFailureIdentityInput(t *testing.T) {
	resetObjectiveFailureTelemetry()
	t.Cleanup(resetObjectiveFailureTelemetry)

	observeAgentLogLine("round 99: arbitrary human text -> failed: this must never become identity, map ff at (1,2)")
	got, terminal := drainObjectiveFailureTelemetry("failed", "build-a", "")
	if len(got) != 0 || terminal != nil {
		t.Fatalf("log prose produced structured telemetry: %+v terminal=%+v", got, terminal)
	}
}
