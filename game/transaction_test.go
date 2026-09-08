package game

import (
	"errors"
	"reflect"
	"testing"
)

type fakeAdapter struct {
	calls []string
	obs   string

	validateErr error
	startErr    error
	executeErr  error
	finishErr   error
	verifyErr   error

	normalizeCalls int
}

func (f *fakeAdapter) Observe() string {
	f.calls = append(f.calls, "observe")
	return f.obs
}

func (f *fakeAdapter) Validate(string) error {
	f.calls = append(f.calls, "validate")
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

func (f *fakeAdapter) SettlePostcondition(string) {
	f.calls = append(f.calls, "settle")
}

func (f *fakeAdapter) VerifyPostcondition(_ string, observation string, _ string) error {
	f.calls = append(f.calls, "verify")
	if f.verifyErr != nil {
		return f.verifyErr
	}
	if observation != "room-b" {
		return errors.New("not in room-b")
	}
	return nil
}

func TestExecuteTransactionOwnsPortableLifecycle(t *testing.T) {
	adapter := &fakeAdapter{obs: "room-a"}
	tx := ExecuteTransaction[string, string, string](adapter, "travel-room-b")

	if tx.ValidationErr != nil || tx.StartBoundaryErr != nil || tx.ExecutionErr != nil || tx.FinishBoundaryErr != nil || tx.PostconditionErr != nil {
		t.Fatalf("transaction errors: %+v", tx)
	}
	if tx.Result != "did:travel-room-b" {
		t.Fatalf("Result = %q", tx.Result)
	}
	if tx.Final != "room-b" {
		t.Fatalf("Final = %q, want room-b", tx.Final)
	}
	want := []string{"validate", "normalize-start", "budget", "execute", "settle", "normalize-finish", "observe", "verify"}
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
	want := []string{"validate", "normalize-start", "budget", "execute", "normalize-finish", "observe"}
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
	want := []string{"validate", "normalize-start", "observe"}
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
	if tx.ExecutionErr != nil || tx.FinishBoundaryErr != nil {
		t.Fatalf("execution/boundary errors = %v / %v", tx.ExecutionErr, tx.FinishBoundaryErr)
	}
}
