package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// redRegistryStory projects Red's Story from blank RAM: every fact is present
// (that is the registration under test) and none is complete.
func redRegistryStory() ProgressState {
	var mem state.Mem
	inv := state.DecodeInventory(&mem)
	return redProgressStateFromRAM(&mem, inv, state.DecodeStoryFacts(&mem, inv))
}

// TestRedProgressionRegistryHasVerifiers is the #1655 registration guard: a
// progression goal may not become executable unless the Red adapter also
// projects the Story fact that positively proves it. Adding an executor
// without its verifier fails here instead of as a silent farm-run success.
func TestRedProgressionRegistryHasVerifiers(t *testing.T) {
	story := redRegistryStory()
	seen := map[ProgressID]int{}
	for _, fact := range story {
		seen[fact.ID]++
	}
	for id := range redProgressionExecutors {
		switch seen[id] {
		case 0:
			t.Errorf("progression %q is executable but has no projected Story verifier", id)
		case 1:
		default:
			t.Errorf("progression %q is projected %d times; want exactly one verifier", id, seen[id])
		}
	}
}

// TestRedProgressionNilExecutorCannotCompleteWithoutFact runs the real Red
// VerifyPostcondition for every registered goal. A skill that returns nil
// without the world changing — a suppressed badge/event/item write — must be
// a structured postcondition_failed, and only the observed fact completes it.
func TestRedProgressionNilExecutorCannotCompleteWithoutFact(t *testing.T) {
	adapter := &redObjectiveAdapter{}
	for id := range redProgressionExecutors {
		o := Objective{Kind: KindProgress, Progress: id}

		suppressed := Observation{Controllable: true, Story: redRegistryStory()}
		err := adapter.VerifyPostcondition(o, Observation{}, suppressed, ObjectiveResult{})
		if !errors.Is(err, ErrObjectivePostconditionFailed) {
			t.Errorf("%q with its fact false: err=%v, want postcondition_failed", id, err)
		}
		if got := classifyObjectiveOutcome(o, err, suppressed); got != OutcomePostconditionFailed {
			t.Errorf("%q with its fact false: outcome=%q, want %q", id, got, OutcomePostconditionFailed)
		}

		observed := Observation{Controllable: true, Story: ProgressState{{ID: id, Complete: true}}}
		if err := adapter.VerifyPostcondition(o, Observation{}, observed, ObjectiveResult{}); err != nil {
			t.Errorf("%q with its fact observed: %v, want completed", id, err)
		}
	}
}

func TestProgressWithoutVerifierIsTerminalUnavailable(t *testing.T) {
	o := Objective{Kind: KindProgress, Progress: ProgressID("unregistered_goal")}
	final := Observation{Controllable: true, Story: ProgressState{{ID: "other_goal", Complete: true}}}
	out, err := verifyObjectivePostcondition(o, Observation{}, final, ObjectiveResult{})
	if out != OutcomePostconditionUnavailable ||
		!errors.Is(err, ErrObjectivePostconditionUnavailable) ||
		!errors.Is(err, ErrProgressVerifierMissing) {
		t.Fatalf("unregistered progress = %q, %v; want unavailable + ErrProgressVerifierMissing", out, err)
	}
	if actionFor(out) != actionStop {
		t.Fatal("a missing progression verifier must be terminal, not replanned")
	}
}

// suppressedProgressGame is the generic-runtime half of the same contract:
// the executor returns nil but the progression write never lands.
type suppressedProgressGame struct{ *fakeObjectiveGame }

func (g suppressedProgressGame) ExecuteOwned(o Objective) (ObjectiveResult, error) {
	g.calls = append(g.calls, "execute")
	g.obs = Observation{MapName: "ROOM_A", Controllable: true, Story: ProgressState{{ID: o.Progress}}}
	return ObjectiveResult{}, nil
}

func TestObjectiveRuntimeSuppressedProgressIsPostconditionFailed(t *testing.T) {
	game := suppressedProgressGame{&fakeObjectiveGame{obs: Observation{MapName: "ROOM_A", Controllable: true}}}
	o := Objective{Kind: KindProgress, Progress: ProgressID("door_unlocked")}

	got, err := executeObjectiveWithAdapter(game, o)
	if !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("err = %v, want postcondition_failed", err)
	}
	if got.Outcome != OutcomePostconditionFailed {
		t.Fatalf("Outcome = %q, want %q", got.Outcome, OutcomePostconditionFailed)
	}
}
