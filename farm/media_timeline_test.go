package farm

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMediaTimelineNormalizesOrderingAndStableIDs(t *testing.T) {
	input := MediaTimeline{
		Run:             MediaRunSummary{RunID: "run-1", Status: "done"},
		Attempt:         2,
		EndFrame:        300,
		FramesPerSecond: GameBoyFramesPerSecond,
		Snapshots: []MediaSnapshot{
			{Frame: 200, Round: 2, Objective: "second"},
			{Frame: 100, Round: 1, Objective: "first"},
		},
		Events: []MediaEvent{
			{Type: "recovery", Frame: 200, Round: 2, Evidence: "outcome:2"},
			{Type: "objective_blocked", Frame: 100, Round: 1, Evidence: "outcome:1"},
		},
	}
	first := input.Normalized()
	second := input.Normalized()
	if got := []uint64{first.Snapshots[0].Frame, first.Snapshots[1].Frame}; !reflect.DeepEqual(got, []uint64{100, 200}) {
		t.Fatalf("snapshot order = %v", got)
	}
	if got := []uint64{first.Events[0].Frame, first.Events[1].Frame}; !reflect.DeepEqual(got, []uint64{100, 200}) {
		t.Fatalf("event order = %v", got)
	}
	if first.Events[0].ID == "" || first.Events[0].ID != second.Events[0].ID || first.Events[1].ID != second.Events[1].ID {
		t.Fatalf("event IDs are not stable: %#v vs %#v", first.Events, second.Events)
	}
	if err := first.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMediaTimelineContextBoundaryLookup(t *testing.T) {
	timeline := MediaTimeline{
		Run:             MediaRunSummary{RunID: "run-boundary"},
		EndFrame:        300,
		FramesPerSecond: GameBoyFramesPerSecond,
		Snapshots:       []MediaSnapshot{{Frame: 100, Round: 1}, {Frame: 200, Round: 2}},
		Events:          []MediaEvent{{Type: "badge_acquired", Frame: 200, Round: 2, Evidence: "badge:Boulder"}},
	}.Normalized()

	if got := timeline.ContextAtFrame(99); got.Snapshot != nil {
		t.Fatalf("frame 99 unexpectedly has snapshot %#v", got.Snapshot)
	}
	if got := timeline.ContextAtFrame(100); got.Snapshot == nil || got.Snapshot.Round != 1 {
		t.Fatalf("frame 100 = %#v", got.Snapshot)
	}
	if got := timeline.ContextAtFrame(199); got.Snapshot == nil || got.Snapshot.Round != 1 {
		t.Fatalf("frame 199 = %#v", got.Snapshot)
	}
	got := timeline.ContextAtFrame(200)
	if got.Snapshot == nil || got.Snapshot.Round != 2 || len(got.Events) != 1 || got.Events[0].Type != "badge_acquired" {
		t.Fatalf("frame 200 context = %#v", got)
	}
	if between := timeline.EventsBetween(100, 200); len(between) != 1 || between[0].Type != "badge_acquired" {
		t.Fatalf("EventsBetween = %#v", between)
	}
}

func TestMediaTimelineMissingOptionalMetadataDegradesCleanly(t *testing.T) {
	artifact, err := NewMediaTimelineArtifact(MediaTimeline{
		Run:      MediaRunSummary{RunID: "old-run"},
		EndFrame: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMediaTimeline(artifact.Data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Run.RunID != "old-run" || decoded.Version != MediaTimelineVersion || decoded.FramesPerSecond <= 0 {
		t.Fatalf("decoded = %#v", decoded)
	}
	if decoded.ContextAtFrame(0).Snapshot != nil {
		t.Fatal("minimal timeline invented a snapshot")
	}
}

func TestMediaTimelineStallRecoveryRoundTripIsDeterministic(t *testing.T) {
	input := MediaTimeline{
		Run:      MediaRunSummary{RunID: "run-recovery", Status: "stuck", Goal: "Earn the Boulder Badge."},
		Attempt:  1,
		EndFrame: 900,
		Snapshots: []MediaSnapshot{
			{Frame: 300, Round: 1, Objective: "go to pewter city", Outcome: "blocked"},
			{Frame: 600, Round: 2, Objective: "recover from repeated objective failures", Outcome: "completed"},
		},
		Events: []MediaEvent{
			{Type: "objective_blocked", Frame: 300, Round: 1, Objective: "go to pewter city", Evidence: "outcome:0"},
			{Type: "failure_recovered", Frame: 600, Round: 2, Objective: "recover from repeated objective failures", Evidence: "outcome:1"},
			{Type: "stalled", Frame: 900, Evidence: "run:stuck"},
		},
	}
	artifact1, err := NewMediaTimelineArtifact(input)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMediaTimeline(artifact1.Data)
	if err != nil {
		t.Fatal(err)
	}
	artifact2, err := NewMediaTimelineArtifact(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	if err := json.Unmarshal(artifact1.Data, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(artifact2.Data, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || artifact1.SHA256 != artifact2.SHA256 {
		t.Fatalf("timeline reconstruction changed\nfirst=%s\nsecond=%s", artifact1.Data, artifact2.Data)
	}
}
