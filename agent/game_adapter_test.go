package agent

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

type fakeObjectiveGame struct {
	calls []string
	obs   Observation

	initialObserveErr error
	finalObserveErr   error
	startBoundaryErr  error
	finishBoundaryErr error
	executeResult     ObjectiveResult
	executeErr        error
	settleErr         error
	verifyErr         error

	observeCalls   int
	normalizeCalls int
}

func (f *fakeObjectiveGame) Observe() (Observation, error) {
	f.calls = append(f.calls, "observe")
	f.observeCalls++
	if f.observeCalls == 1 && f.initialObserveErr != nil {
		return f.obs, f.initialObserveErr
	}
	if f.observeCalls > 1 && f.finalObserveErr != nil {
		return f.obs, f.finalObserveErr
	}
	return f.obs, nil
}

func (f *fakeObjectiveGame) Validate(_ Objective, initial Observation) error {
	f.calls = append(f.calls, "validate")
	if initial.MapName != f.obs.MapName {
		return errors.New("validation did not receive initial observation")
	}
	return nil
}

func (f *fakeObjectiveGame) NormalizeBoundary() error {
	f.normalizeCalls++
	if f.normalizeCalls == 1 {
		f.calls = append(f.calls, "normalize-start")
		return f.startBoundaryErr
	}
	f.calls = append(f.calls, "normalize-finish")
	return f.finishBoundaryErr
}

func (f *fakeObjectiveGame) ExecuteOwned(o Objective) (ObjectiveResult, error) {
	f.calls = append(f.calls, "execute")
	if o.Kind == KindProgress {
		f.obs = Observation{
			MapName:      "ROOM_A",
			Controllable: true,
			Story:        ProgressState{{ID: o.Progress, Complete: true}},
		}
	} else {
		f.obs = Observation{MapName: "ROOM_B", X: 2, Y: 3, Controllable: true}
	}
	return f.executeResult, f.executeErr
}

func (f *fakeObjectiveGame) WithinObjectiveBudget(_ Objective, fn func() error) error {
	f.calls = append(f.calls, "budget")
	return fn()
}

func (f *fakeObjectiveGame) SettlePostcondition(Objective) error {
	f.calls = append(f.calls, "settle")
	return f.settleErr
}

func (f *fakeObjectiveGame) VerifyPostcondition(o Objective, initial, final Observation, result ObjectiveResult) error {
	f.calls = append(f.calls, "verify")
	if f.verifyErr != nil {
		return f.verifyErr
	}
	if o.Kind == KindProgress {
		_, err := verifyObjectivePostcondition(o, initial, final, result)
		return err
	}
	if initial.MapName != "ROOM_A" {
		return errors.New("fake game did not receive initial ROOM_A state")
	}
	if final.MapName != "ROOM_B" || !final.Controllable {
		return errors.New("fake game did not reach ROOM_B")
	}
	return nil
}

func (f *fakeObjectiveGame) NormalizeFailure(phase gameruntime.FailurePhase, err error, _ Observation) gameruntime.Failure {
	out := OutcomeUnknownFailure
	switch {
	case phase == gameruntime.FailurePhaseInitialObservation || phase == gameruntime.FailurePhaseFinalObservation:
		out = OutcomeControllerUncertain
	case errors.Is(err, ErrObjectiveBoundaryChoice):
		out = OutcomeChoiceRequired
	case errors.Is(err, ErrObjectiveBoundaryDirty):
		out = OutcomeStabilizationFailed
	case errors.Is(err, ErrObjectivePostconditionUnavailable):
		out = OutcomePostconditionUnavailable
	case errors.Is(err, ErrObjectivePostconditionFailed):
		out = OutcomePostconditionFailed
	}
	return gameruntime.Failure{
		Phase:       phase,
		Class:       failureClassForOutcome(out),
		Cause:       "fake_failure",
		Recoverable: actionFor(out) == actionReplan,
	}
}

func (f *fakeObjectiveGame) CaptureFailure(Objective, error) error {
	f.calls = append(f.calls, "capture")
	return nil
}

func countCall(calls []string, want string) int {
	n := 0
	for _, call := range calls {
		if call == want {
			n++
		}
	}
	return n
}

func TestObjectiveRuntimeUsesGameAdapterWithoutEmulator(t *testing.T) {
	adapter := &fakeObjectiveGame{
		obs: Observation{MapName: "ROOM_A", Controllable: true},
	}
	o := Objective{Kind: KindGoTo, Place: "room-b"}

	got, err := executeObjectiveWithAdapter(adapter, o)
	if err != nil {
		t.Fatalf("executeObjectiveWithAdapter: %v", err)
	}
	if got.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q, want completed", got.Outcome)
	}
	if got.Final.MapName != "ROOM_B" || got.Final.X != 2 || got.Final.Y != 3 {
		t.Fatalf("Final = %+v", got.Final)
	}
	wantCalls := []string{"observe", "validate", "normalize-start", "budget", "execute", "settle", "normalize-finish", "observe", "verify"}
	if !reflect.DeepEqual(adapter.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", adapter.calls, wantCalls)
	}
}

func TestObjectiveRuntimeCompletesGenericProgressionWithoutRed(t *testing.T) {
	adapter := &fakeObjectiveGame{
		obs: Observation{MapName: "ROOM_A", Controllable: true},
	}
	o := Objective{Kind: KindProgress, Progress: ProgressID("door_unlocked")}

	got, err := executeObjectiveWithAdapter(adapter, o)
	if err != nil {
		t.Fatalf("executeObjectiveWithAdapter: %v", err)
	}
	if got.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q, want completed", got.Outcome)
	}
	if !got.Final.Story.Has(o.Progress) {
		t.Fatalf("final story = %+v, want %q complete", got.Final.Story, o.Progress)
	}
}

func TestObjectiveRuntimeMachineUnusableSkipsCaptureAndFinish(t *testing.T) {
	execErr := fmt.Errorf("link stalled: %w", gameruntime.ErrMachineUnusable)
	adapter := &fakeObjectiveGame{
		obs:        Observation{MapName: "ROOM_A", Controllable: true},
		executeErr: execErr,
	}
	o := Objective{Kind: KindCatch, Species: "vulpix", Intent: "dex-virtual-version-assisted"}

	got, err := executeObjectiveWithAdapter(adapter, o)
	if !errors.Is(err, gameruntime.ErrMachineUnusable) {
		t.Fatalf("error = %v, want machine-unusable identity", err)
	}
	if got.Final.MapName != "ROOM_A" {
		t.Fatalf("Final = %+v, want the initial observation", got.Final)
	}
	for _, call := range []string{"normalize-finish", "settle", "capture"} {
		if countCall(adapter.calls, call) != 0 {
			t.Fatalf("call %q ran after a poisoned machine: %#v", call, adapter.calls)
		}
	}
}

func TestObjectiveRuntimePreservesAdapterBlockedOutcome(t *testing.T) {
	blockedErr := errors.New("door locked")
	adapter := &fakeObjectiveGame{
		obs:           Observation{MapName: "ROOM_A", Controllable: true},
		executeResult: ObjectiveResult{Outcome: OutcomeBlocked},
		executeErr:    blockedErr,
	}
	o := Objective{Kind: KindGoTo, Place: "room-b"}

	got, err := executeObjectiveWithAdapter(adapter, o)
	if !errors.Is(err, blockedErr) {
		t.Fatalf("error = %v, want blocked error identity", err)
	}
	if got.Outcome != OutcomeBlocked {
		t.Fatalf("Outcome = %q, want blocked", got.Outcome)
	}
	if got.Failure == nil || got.Failure.Class != gameruntime.FailureClassBlocked || !got.Failure.Recoverable {
		t.Fatalf("Failure = %+v, want recoverable blocked failure", got.Failure)
	}
	if got.Final.MapName != "ROOM_B" {
		t.Fatalf("Final = %+v, want transaction-owned ROOM_B observation", got.Final)
	}
	if countCall(adapter.calls, "verify") != 0 {
		t.Fatalf("verify called after blocked execution: %#v", adapter.calls)
	}
}

func TestObjectiveRuntimeDirtyFinishPreservesFinalAndStops(t *testing.T) {
	adapter := &fakeObjectiveGame{
		obs:               Observation{MapName: "ROOM_A", Controllable: true},
		finishBoundaryErr: ErrObjectiveBoundaryDirty,
	}
	o := Objective{Kind: KindGoTo, Place: "room-b"}

	got, err := executeObjectiveWithAdapter(adapter, o)
	if !errors.Is(err, ErrObjectiveBoundaryDirty) {
		t.Fatalf("error = %v, want dirty-boundary identity", err)
	}
	if got.Outcome != OutcomeStabilizationFailed {
		t.Fatalf("Outcome = %q, want stabilization_failed", got.Outcome)
	}
	if got.Final.MapName != "ROOM_B" || got.Final.X != 2 || got.Final.Y != 3 {
		t.Fatalf("Final = %+v, want transaction final observation preserved", got.Final)
	}
	if countCall(adapter.calls, "verify") != 0 {
		t.Fatalf("verify called after dirty finish: %#v", adapter.calls)
	}
}

func TestObjectiveRuntimeChoiceFinishPreservesFinalAndRequiresChoice(t *testing.T) {
	adapter := &fakeObjectiveGame{
		obs:               Observation{MapName: "ROOM_A", Controllable: true},
		finishBoundaryErr: ErrObjectiveBoundaryChoice,
	}
	o := Objective{Kind: KindGoTo, Place: "room-b"}

	got, err := executeObjectiveWithAdapter(adapter, o)
	if !errors.Is(err, ErrObjectiveBoundaryChoice) {
		t.Fatalf("error = %v, want choice-boundary identity", err)
	}
	if got.Outcome != OutcomeChoiceRequired {
		t.Fatalf("Outcome = %q, want choice_required", got.Outcome)
	}
	if got.Final.MapName != "ROOM_B" || got.Final.X != 2 || got.Final.Y != 3 {
		t.Fatalf("Final = %+v, want transaction final observation preserved", got.Final)
	}
	if countCall(adapter.calls, "verify") != 0 {
		t.Fatalf("verify called while a choice remains open: %#v", adapter.calls)
	}
}

func TestObjectiveRuntimeInitialObservationFailureIsControllerUncertain(t *testing.T) {
	observeErr := errors.New("cannot decode semantic state")
	adapter := &fakeObjectiveGame{
		obs:               Observation{MapName: "ROOM_A"},
		initialObserveErr: observeErr,
	}
	o := Objective{Kind: KindGoTo, Place: "room-b"}

	got, err := executeObjectiveWithAdapter(adapter, o)
	if !errors.Is(err, observeErr) {
		t.Fatalf("error = %v, want observation error identity", err)
	}
	if got.Outcome != OutcomeControllerUncertain {
		t.Fatalf("Outcome = %q, want controller_uncertain", got.Outcome)
	}
	if got.Initial != nil {
		t.Fatalf("Initial = %+v, want nil when initial observation was unavailable", got.Initial)
	}
	if countCall(adapter.calls, "validate") != 0 || countCall(adapter.calls, "execute") != 0 {
		t.Fatalf("gameplay continued after failed initial observation: %#v", adapter.calls)
	}
}

func TestObjectiveRuntimeSettleFailureIsPostconditionUnavailable(t *testing.T) {
	settleErr := errors.New("warp never settled")
	adapter := &fakeObjectiveGame{
		obs:       Observation{MapName: "ROOM_A", Controllable: true},
		settleErr: settleErr,
	}
	o := Objective{Kind: KindGoTo, Place: "room-b"}

	got, err := executeObjectiveWithAdapter(adapter, o)
	if !errors.Is(err, settleErr) {
		t.Fatalf("error = %v, want settle error identity", err)
	}
	if got.Outcome != OutcomePostconditionUnavailable {
		t.Fatalf("Outcome = %q, want postcondition_unavailable", got.Outcome)
	}
	if countCall(adapter.calls, "verify") != 0 {
		t.Fatalf("verify called after settle failure: %#v", adapter.calls)
	}
	if countCall(adapter.calls, "normalize-finish") != 1 || countCall(adapter.calls, "observe") != 2 {
		t.Fatalf("settle failure did not still own finish boundary/final observation: %#v", adapter.calls)
	}
}

func TestObjectiveRuntimeFinalObservationFailureIsControllerUncertain(t *testing.T) {
	observeErr := errors.New("final state unreadable")
	adapter := &fakeObjectiveGame{
		obs:             Observation{MapName: "ROOM_A", Controllable: true},
		finalObserveErr: observeErr,
	}
	o := Objective{Kind: KindGoTo, Place: "room-b"}

	got, err := executeObjectiveWithAdapter(adapter, o)
	if !errors.Is(err, observeErr) {
		t.Fatalf("error = %v, want observation error identity", err)
	}
	if got.Outcome != OutcomeControllerUncertain {
		t.Fatalf("Outcome = %q, want controller_uncertain", got.Outcome)
	}
	if countCall(adapter.calls, "verify") != 0 {
		t.Fatalf("verify called without a trustworthy final observation: %#v", adapter.calls)
	}
}
