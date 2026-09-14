package agent

import (
	"errors"
	"reflect"
	"testing"
)

func TestRunGoalPolicyCompletionPrecedesRoundBudget(t *testing.T) {
	policy, err := newRunGoalPolicy(nil, Budget{Goal: "badges:1", MaxRounds: 1})
	if err != nil {
		t.Fatal(err)
	}
	obs := Observation{Badges: []string{"boulder"}}
	got := policy.startRound(nil, obs, 2, "", 0)
	if got.Stop != StopDone {
		t.Fatalf("startRound stop = %v; want StopDone before round budget", got.Stop)
	}
	if got.Status == nil || !got.Status.Complete {
		t.Fatalf("goal status = %+v; want complete", got.Status)
	}
}

func TestRunGoalPolicyRoundBudgetStopsIncompleteGoal(t *testing.T) {
	policy, err := newRunGoalPolicy(nil, Budget{Goal: "badges:1", MaxRounds: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := policy.startRound(nil, Observation{}, 2, "", 0)
	if got.Stop != StopBudget {
		t.Fatalf("startRound stop = %v; want StopBudget", got.Stop)
	}
	if got.Status == nil || got.Status.Complete {
		t.Fatalf("goal status = %+v; want incomplete deterministic status", got.Status)
	}
}

func TestRunGoalPolicyRejectsPlannerDoneBeforeDeterministicGoal(t *testing.T) {
	policy, err := newRunGoalPolicy(nil, Budget{Goal: "badges:1"})
	if err != nil {
		t.Fatal(err)
	}
	got := policy.plannerDone(nil, Observation{}, 1, "", 0, nil)
	if got.Stop != StopError {
		t.Fatalf("plannerDone stop = %v; want StopError", got.Stop)
	}
	if !errors.Is(got.Err, ErrGoalIncomplete) {
		t.Fatalf("plannerDone err = %v; want ErrGoalIncomplete", got.Err)
	}
}

func TestRunKnowledgePolicyRequestsReplanOnlyForNewSemanticFacts(t *testing.T) {
	known := NewKnowledge(nil)
	initial := Observation{Map: 1}
	policy := newRunKnowledgePolicy(initial, known)

	changed := Observation{
		Map:    2,
		Badges: []string{"boulder"},
		Events: []string{"oak_parcel"},
	}
	got := policy.roundBoundary(2, changed, known, []uint8{3})
	want := []string{"badge_changed", "story_changed"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("replan reasons = %v; want %v", got, want)
	}
	if !known.Visited[2] || !known.Visited[3] {
		t.Fatalf("visited = %v; want current and sampled maps recorded", known.Visited)
	}

	got = policy.roundBoundary(3, changed, known, nil)
	if len(got) != 0 {
		t.Fatalf("unchanged facts requested replans: %v", got)
	}
}

func TestRunEngineBeginRoundCompletionSkipsKnowledgeAndWatchdogs(t *testing.T) {
	known := NewKnowledge(nil)
	initial := Observation{Map: 1, Badges: []string{"boulder"}}
	goal, err := newRunGoalPolicy(nil, Budget{Goal: "badges:1", MaxRounds: 1})
	if err != nil {
		t.Fatal(err)
	}
	engine := newRunEngine(Budget{StagnationAfter: 1}, Plan{}, initial, known, goal)
	called := false
	got := engine.beginRound(nil, 2, initial, known, func() []uint8 {
		called = true
		return []uint8{2}
	}, "", 0)
	if got.Stop != StopDone {
		t.Fatalf("beginRound stop = %v; want StopDone", got.Stop)
	}
	if called {
		t.Fatal("beginRound consumed sampled maps after terminal goal completion")
	}
	if known.Visited[2] {
		t.Fatal("terminal goal completion mutated knowledge before stopping")
	}
}
