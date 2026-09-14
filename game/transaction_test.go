package game

import (
	"errors"
	"reflect"
	"testing"
)

type fakeAdapter struct {
	calls []string
	obs   string

	initialObserveErr error
	finalObserveErr   error
	validateErr       error
	startErr          error
	executeErr        error
	settleErr         error
	finishErr         error
	verifyErr         error

	observeCalls   int
	normalizeCalls int
}

func (f *fakeAdapter) Observe() (string, error) {
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

func (f *fakeAdapter) Validate(_ string, observation string) error {
	f.calls = append(f.calls, "validate")
	if observation != f.obs {
		return errors.New("validation did not receive the initial observation")
	}
	return f.validateErr
}

func (f *fakeAdapter) NormalizeBoundary() error {
	f.normalizeCalls++
	if f.normalizeCalls == 1 {
		f.calls = append(f.calls, "normalize-start")
		return f.startErr
	}
	f.calls = append(f.calls, "normalize-finish")
	return f.finishErr
}

func (f *fakeAdapter) ExecuteOwned(objective string) (string, error) {
	f.calls = append(f.calls, "execute")
	if f.executeErr == nil {
		f.obs = "room-b"
	}
	return "did:" + objective, f.executeErr
}

func (f *fakeAdapter) WithinObjectiveBudget(_ string, fn func() error) error {
	f.calls = append(f.calls, "budget")
	return fn()
}

func (f *fakeAdapter) SettlePostcondition(string) error {
	f.calls = append(f.calls, "settle")
	return f.settleErr
}

func (f *fakeAdapter) VerifyPostcondition(_ string, initial, final, _ string) error {
	f.calls = append(f.calls, "verify")
	if f.verifyErr != nil {
		return f.verifyErr
	}
	if initial != "room-a" {
		return errors.New("verifier did not receive initial observation")
	}
	if final != "room-b" {
		return errors.New("not in room-b")
	}
	return nil
}

func TestExecuteTransactionOwnsPortableLifecycle(t *testing.T) {
	adapter := &fakeAdapter{obs: "room-a"}
	tx := ExecuteTransaction[string, string, string](adapter, "travel-room-b")

	if tx.InitialObservationErr != nil || tx.ValidationErr != nil || tx.StartBoundaryErr != nil || tx.ExecutionErr != nil || tx.SettleErr != nil || tx.FinishBoundaryErr != nil || tx.FinalObservationErr != nil || tx.PostconditionErr != nil {
		t.Fatalf("transaction errors: %+v", tx)
	}
	if tx.Initial != "room-a" {
		t.Fatalf("Initial = %q, want room-a", tx.Initial)
	}
	if tx.Result != "did:travel-room-b" {
		t.Fatalf("Result = %q", tx.Result)
	}
	if tx.Final != "room-b" {
		t.Fatalf("Final = %q, want room-b", tx.Final)
	}
	want := []string{"observe", "validate", "normalize-start", "budget", "execute", "settle", "normalize-finish", "observe", "verify"}
	if !reflect.DeepEqual(adapter.calls, want) {
		t.Fatalf("calls = %#v, want %#v", adapter.calls, want)
	}
}

func TestExecuteTransactionExecutionFailureStillOwnsFinishBoundary(t *testing.T) {
	execErr := errors.New("blocked")
	adapter := &fakeAdapter{obs: "room-a", executeErr: execErr}
	tx := ExecuteTransaction[string, string, string](adapter, "travel-room-b")

	if !errors.Is(tx.ExecutionErr, execErr) {
		t.Fatalf("ExecutionErr = %v, want %v", tx.ExecutionErr, execErr)
	}
	if tx.PostconditionErr != nil {
		t.Fatalf("PostconditionErr = %v, want nil after execution failure", tx.PostconditionErr)
	}
	want := []string{"observe", "validate", "normalize-start", "budget", "execute", "normalize-finish", "observe"}
	if !reflect.DeepEqual(adapter.calls, want) {
		t.Fatalf("calls = %#v, want %#v", adapter.calls, want)
	}
}

func TestExecuteTransactionStartInvariantStopsBeforeGameplay(t *testing.T) {
	startErr := errors.New("dirty start")
	adapter := &fakeAdapter{obs: "menu", startErr: startErr}
	tx := ExecuteTransaction[string, string, string](adapter, "travel-room-b")

	if !errors.Is(tx.StartBoundaryErr, startErr) {
		t.Fatalf("StartBoundaryErr = %v, want %v", tx.StartBoundaryErr, startErr)
	}
	want := []string{"observe", "validate", "normalize-start", "observe"}
	if !reflect.DeepEqual(adapter.calls, want) {
		t.Fatalf("calls = %#v, want %#v", adapter.calls, want)
	}
}

func TestExecuteTransactionValidationFailureUsesInitialObservation(t *testing.T) {
	validationErr := errors.New("bad objective")
	adapter := &fakeAdapter{obs: "room-a", validateErr: validationErr}
	tx := ExecuteTransaction[string, string, string](adapter, "bad")

	if !errors.Is(tx.ValidationErr, validationErr) {
		t.Fatalf("ValidationErr = %v, want %v", tx.ValidationErr, validationErr)
	}
	if tx.Initial != "room-a" || tx.Final != "room-a" {
		t.Fatalf("Initial/Final = %q/%q, want room-a/room-a", tx.Initial, tx.Final)
	}
	want := []string{"observe", "validate"}
	if !reflect.DeepEqual(adapter.calls, want) {
		t.Fatalf("calls = %#v, want %#v", adapter.calls, want)
	}
}

func TestExecuteTransactionPostconditionFailureIsSeparate(t *testing.T) {
	postErr := errors.New("wrong destination")
	adapter := &fakeAdapter{obs: "room-a", verifyErr: postErr}
	tx := ExecuteTransaction[string, string, string](adapter, "travel-room-b")

	if !errors.Is(tx.PostconditionErr, postErr) {
		t.Fatalf("PostconditionErr = %v, want %v", tx.PostconditionErr, postErr)
	}
	if tx.ExecutionErr != nil || tx.SettleErr != nil || tx.FinishBoundaryErr != nil || tx.FinalObservationErr != nil {
		t.Fatalf("execution/settle/boundary/observation errors = %v / %v / %v / %v", tx.ExecutionErr, tx.SettleErr, tx.FinishBoundaryErr, tx.FinalObservationErr)
	}
}

func TestExecuteTransactionInitialObservationFailureStopsBeforeValidation(t *testing.T) {
	observeErr := errors.New("cannot decode state")
	adapter := &fakeAdapter{obs: "room-a", initialObserveErr: observeErr}
	tx := ExecuteTransaction[string, string, string](adapter, "travel-room-b")

	if !errors.Is(tx.InitialObservationErr, observeErr) {
		t.Fatalf("InitialObservationErr = %v, want %v", tx.InitialObservationErr, observeErr)
	}
	want := []string{"observe"}
	if !reflect.DeepEqual(adapter.calls, want) {
		t.Fatalf("calls = %#v, want %#v", adapter.calls, want)
	}
}

func TestExecuteTransactionSettleFailureStillOwnsFinishBoundary(t *testing.T) {
	settleErr := errors.New("transition never settled")
	adapter := &fakeAdapter{obs: "room-a", settleErr: settleErr}
	tx := ExecuteTransaction[string, string, string](adapter, "travel-room-b")

	if !errors.Is(tx.SettleErr, settleErr) {
		t.Fatalf("SettleErr = %v, want %v", tx.SettleErr, settleErr)
	}
	if tx.PostconditionErr != nil {
		t.Fatalf("PostconditionErr = %v, want nil after settle failure", tx.PostconditionErr)
	}
	want := []string{"observe", "validate", "normalize-start", "budget", "execute", "settle", "normalize-finish", "observe"}
	if !reflect.DeepEqual(adapter.calls, want) {
		t.Fatalf("calls = %#v, want %#v", adapter.calls, want)
	}
}

func TestExecuteTransactionFinalObservationFailureSkipsVerification(t *testing.T) {
	observeErr := errors.New("final state unreadable")
	adapter := &fakeAdapter{obs: "room-a", finalObserveErr: observeErr}
	tx := ExecuteTransaction[string, string, string](adapter, "travel-room-b")

	if !errors.Is(tx.FinalObservationErr, observeErr) {
		t.Fatalf("FinalObservationErr = %v, want %v", tx.FinalObservationErr, observeErr)
	}
	if tx.PostconditionErr != nil {
		t.Fatalf("PostconditionErr = %v, want nil when final observation failed", tx.PostconditionErr)
	}
	want := []string{"observe", "validate", "normalize-start", "budget", "execute", "settle", "normalize-finish", "observe"}
	if !reflect.DeepEqual(adapter.calls, want) {
		t.Fatalf("calls = %#v, want %#v", adapter.calls, want)
	}
}
