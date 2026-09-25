package agent_test

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
	gameruntime "github.com/maestroi/pokepilot/game"
)

type portableFakeAdapter struct {
	obs agent.Observation
}

func (f *portableFakeAdapter) Observe() (agent.Observation, error) {
	return f.obs, nil
}

func (f *portableFakeAdapter) Validate(o agent.Objective, _ agent.Observation) error {
	return o.Validate()
}

func (f *portableFakeAdapter) NormalizeBoundary() error {
	return nil
}

func (f *portableFakeAdapter) ExecuteOwned(o agent.Objective) (agent.ObjectiveResult, error) {
	f.obs.Location = o.Place
	f.obs.Controllable = true
	return agent.ObjectiveResult{}, nil
}

func (f *portableFakeAdapter) WithinObjectiveBudget(_ agent.Objective, fn func() error) error {
	return fn()
}

func (f *portableFakeAdapter) SettlePostcondition(agent.Objective) error {
	return nil
}

func (f *portableFakeAdapter) VerifyPostcondition(o agent.Objective, _, final agent.Observation, _ agent.ObjectiveResult) error {
	if final.Location != o.Place {
		return &portableVerificationError{got: final.Location, want: o.Place}
	}
	return nil
}

func (f *portableFakeAdapter) NormalizeFailure(phase gameruntime.FailurePhase, _ error, _ agent.Observation) gameruntime.Failure {
	return gameruntime.Failure{Phase: phase, Class: gameruntime.FailureClassUnknown, Cause: "portable_fake"}
}

func (f *portableFakeAdapter) CaptureFailure(agent.Objective, error) error {
	return nil
}

type portableVerificationError struct {
	got  agent.PlaceID
	want agent.PlaceID
}

func (e *portableVerificationError) Error() string {
	return "portable fake destination mismatch"
}

func TestExecuteWithAdapterNeedsNoRedRuntime(t *testing.T) {
	adapter := &portableFakeAdapter{
		obs: agent.Observation{Location: "room-a", Controllable: true},
	}
	objective := agent.Objective{Kind: agent.KindGoTo, Place: "room-b"}

	result, err := agent.ExecuteWithAdapter(adapter, objective)
	if err != nil {
		t.Fatalf("ExecuteWithAdapter: %v", err)
	}
	if result.Outcome != agent.OutcomeCompleted {
		t.Fatalf("outcome = %q, want completed", result.Outcome)
	}
	if result.Final.Location != "room-b" {
		t.Fatalf("final location = %q, want room-b", result.Final.Location)
	}
}
