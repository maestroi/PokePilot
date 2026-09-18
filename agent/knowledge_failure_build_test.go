package agent

import (
	"errors"
	"testing"
)

// TestFailureTallyResetsOnBuildChange pins the fix for a planner strategist
// permanently avoiding an objective whose bug was already fixed: a failure
// tally recorded under an older build is stale evidence, not proof the step
// is still broken (docs/ARCHITECTURE.md: repeats only count "same build").
func TestFailureTallyResetsOnBuildChange(t *testing.T) {
	o := Objective{Kind: KindProgress, Progress: "saffron_gate_open"}
	err := errors.New("unexpected menu while waiting")

	known := NewKnowledge(nil)
	known.Build = "build-a"
	for i := 0; i < 5; i++ {
		known.Failed(o, err)
	}
	if f, _ := known.ordinaryFailure(o); f.Times != 5 {
		t.Fatalf("same-build failures: got %d, want 5", f.Times)
	}

	known.Build = "build-b"
	known.Failed(o, err)
	if f, _ := known.ordinaryFailure(o); f.Times != 1 {
		t.Fatalf("failure tally after build change: got %d, want 1 (reset)", f.Times)
	}

	known.Failed(o, err)
	if f, _ := known.ordinaryFailure(o); f.Times != 2 {
		t.Fatalf("same-build failures after reset: got %d, want 2", f.Times)
	}
}

// TestFailureTallyUnaffectedWithoutKnownBuild preserves pre-feature behavior
// when the caller never sets Knowledge.Build (e.g. dev builds, older tests).
func TestFailureTallyUnaffectedWithoutKnownBuild(t *testing.T) {
	o := Objective{Kind: KindProgress, Progress: "saffron_gate_open"}
	err := errors.New("unexpected menu while waiting")

	known := NewKnowledge(nil)
	for i := 0; i < 3; i++ {
		known.Failed(o, err)
	}
	if f, _ := known.ordinaryFailure(o); f.Times != 3 {
		t.Fatalf("failures without a known build: got %d, want 3", f.Times)
	}
}
