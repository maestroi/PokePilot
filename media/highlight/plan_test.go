package highlight

import (
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func testTimeline(events ...farm.MediaEvent) farm.MediaTimeline {
	return farm.MediaTimeline{
		Run: farm.MediaRunSummary{RunID: "run-1"},
		Attempt: 1,
		EndFrame: 60 * 600,
		FramesPerSecond: 60,
		Events: events,
	}.Normalized()
}

func TestBuildSelectsRequiredHighlightEvents(t *testing.T) {
	plan := Build([]farm.MediaTimeline{testTimeline(
		farm.MediaEvent{Type: "badge_acquired", Frame: 60 * 30, Summary: "Boulder Badge"},
		farm.MediaEvent{Type: "blackout", Frame: 60 * 90},
		farm.MediaEvent{Type: "evolution", Frame: 60 * 150},
		farm.MediaEvent{Type: "run_finished", Frame: 60 * 240},
	)}, DefaultPolicy())
	if len(plan.Windows) != 4 {
		t.Fatalf("windows=%d, want 4: %+v", len(plan.Windows), plan.Windows)
	}
	got := map[string]bool{}
	for _, window := range plan.Windows {
		for _, event := range window.Events { got[event.Type] = true }
	}
	for _, kind := range []string{"badge_acquired", "blackout", "evolution", "run_finished"} {
		if !got[kind] { t.Fatalf("missing %s in %+v", kind, plan.Windows) }
	}
}

func TestBuildClampsPreAndPostRoll(t *testing.T) {
	timeline := testTimeline(
		farm.MediaEvent{Type: "badge_acquired", Frame: 60 * 5},
		farm.MediaEvent{Type: "run_finished", Frame: 60*600 - 1},
	)
	plan := Build([]farm.MediaTimeline{timeline}, DefaultPolicy())
	if len(plan.Windows) != 2 { t.Fatalf("windows=%d", len(plan.Windows)) }
	if plan.Windows[0].StartFrame != 0 { t.Fatalf("first start=%d, want 0", plan.Windows[0].StartFrame) }
	if got := plan.Windows[1].EndFrame; got != timeline.EndFrame { t.Fatalf("last end=%d want %d", got, timeline.EndFrame) }
}

func TestBuildMergesOverlappingAndNearbyEvents(t *testing.T) {
	policy := DefaultPolicy()
	policy.MergeGapMS = 5000
	plan := Build([]farm.MediaTimeline{testTimeline(
		farm.MediaEvent{Type: "gym_battle", Frame: 60 * 100},
		farm.MediaEvent{Type: "badge_acquired", Frame: 60 * 104},
	)}, policy)
	if len(plan.Windows) != 1 { t.Fatalf("windows=%d want 1: %+v", len(plan.Windows), plan.Windows) }
	if len(plan.Windows[0].Events) != 2 { t.Fatalf("merged events=%d want 2", len(plan.Windows[0].Events)) }
	if plan.Windows[0].Priority != 100 { t.Fatalf("priority=%d want 100", plan.Windows[0].Priority) }
}

func TestBuildDurationCapUsesDeterministicPriority(t *testing.T) {
	policy := DefaultPolicy()
	policy.TargetDurationMS = 35_000
	policy.MergeGapMS = 0
	timeline := testTimeline(
		farm.MediaEvent{Type: "checkpoint", Frame: 60 * 60},
		farm.MediaEvent{Type: "badge_acquired", Frame: 60 * 180},
		farm.MediaEvent{Type: "evolution", Frame: 60 * 300},
	)
	a := Build([]farm.MediaTimeline{timeline}, policy)
	b := Build([]farm.MediaTimeline{timeline}, policy)
	if a.Hash != b.Hash { t.Fatalf("same inputs changed hash: %s != %s", a.Hash, b.Hash) }
	if a.DurationMS > policy.TargetDurationMS { t.Fatalf("duration=%d exceeds target=%d", a.DurationMS, policy.TargetDurationMS) }
	if len(a.Windows) != 1 || len(a.Windows[0].Events) != 1 || a.Windows[0].Events[0].Type != "badge_acquired" {
		t.Fatalf("priority cap selected %+v", a.Windows)
	}
}

func TestBuildOrdersSelectedWindowsChronologically(t *testing.T) {
	policy := DefaultPolicy()
	policy.TargetDurationMS = 120_000
	plan := Build([]farm.MediaTimeline{testTimeline(
		farm.MediaEvent{Type: "evolution", Frame: 60 * 300},
		farm.MediaEvent{Type: "badge_acquired", Frame: 60 * 100},
	)}, policy)
	if len(plan.Windows) != 2 { t.Fatalf("windows=%d", len(plan.Windows)) }
	if plan.Windows[0].StartFrame >= plan.Windows[1].StartFrame { t.Fatalf("not chronological: %+v", plan.Windows) }
	if plan.Windows[0].Index != 0 || plan.Windows[1].Index != 1 { t.Fatalf("indexes not stable: %+v", plan.Windows) }
}
