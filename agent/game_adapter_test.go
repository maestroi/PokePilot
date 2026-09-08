package agent

import (
	"errors"
	"reflect"
	"testing"
)

type fakeObjectiveGame struct {
	calls []string
	obs   Observation

	executeResult ObjectiveResult
	executeErr    error
	verifyErr     error
}

func (f *fakeObjectiveGame) Observe() Observation {
	f.calls = append(f.calls, "observe")
	return f.obs
}

func (f *fakeObjectiveGame) Validate(_ Objective, initial Observation) error {
	f.calls = append(f.calls, "validate")
	if initial.MapName != f.obs.MapName {
		return errors.New("validation did not receive initial observation")
	}
	return nil
}

func (f *fakeObjectiveGame) NormalizeBoundary() error {
	if len(f.calls) > 0 && f.calls[len(f.calls)-1] == "settle" {
		f.calls = append(f.calls, "normalize-finish")
	} else if countCall(f.calls, "normalize-start") == 0 {
		f.calls = append(f.calls, "normalize-start")
	} else {
		f.calls = append(f.calls, "normalize-finish")
	}
	return nil
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

func (f *fakeObjectiveGame) SettlePostcondition(Objective) {
	f.calls = append(f.calls, "settle")
}

func (f *fakeObjectiveGame) VerifyPostcondition(o Objective, final Observation, _ ObjectiveResult) error {
	f.calls = append(f.calls, "verify")
	if f.verifyErr != nil {
		return f.verifyErr
	}
	if o.Kind == KindProgress {
		_, err := objectivePostcondition(o, final)
		return err
	}
	if final.MapName != "ROOM_B" || !final.Controllable {
		return errors.New("fake game did not reach ROOM_B")
	}
	return nil
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
	if countCall(adapter.calls, "verify") != 0 {
		t.Fatalf("verify called after blocked execution: %#v", adapter.calls)
	}
}
