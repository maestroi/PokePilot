package main

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

func TestDrainMediaTimelineArtifactUsesReplayRelativeFramesAndSemanticEvents(t *testing.T) {
	resetMediaTimelineTelemetry()
	t.Cleanup(resetMediaTimelineTelemetry)

	initial := agent.Observation{
		Map: 1, Location: agent.PlaceID("pewter-city"), MapName: "Pewter City", X: 3, Y: 4,
		Party: []agent.PartyMon{{Species: agent.SpeciesID("squirtle"), Level: 12, HP: 30, MaxHP: 30}},
	}
	blocked := initial
	blocked.X = 8
	afterGym := blocked
	afterGym.Badges = []string{"Boulder Badge"}
	final := afterGym
	final.Party = []agent.PartyMon{{Species: agent.SpeciesID("wartortle"), Level: 16, HP: 42, MaxHP: 42}}

	res := agent.Result{
		Stop:       agent.StopDone,
		Rounds:     3,
		Initial:    initial,
		StartFrame: 120,
		Final:      final,
		FinalFrame: 480,
		Outcomes: []agent.ObjectiveResult{
			{
				Objective: agent.Objective{Kind: agent.KindGoTo, Place: agent.PlaceID("pewter-city")},
				Outcome:   agent.OutcomeBlocked, Summary: "route was blocked", Final: blocked, Recovered: true,
			},
			{
				Objective: agent.Objective{Kind: agent.KindGym, Place: agent.PlaceID("pewter-gym")},
				Outcome:   agent.OutcomeCompleted, Summary: "gym won", Final: afterGym,
			},
			{
				Objective: agent.Objective{Kind: agent.KindTrain, Intent: "dex-evolution"},
				Outcome:   agent.OutcomeCompleted, Summary: "trained", Final: final,
			},
		},
		OutcomeTimings: []agent.ObjectiveTiming{
			{Frame: 220, Round: 1},
			{Frame: 320, Round: 2},
			{Frame: 420, Round: 3},
		},
	}
	captureMediaTimelineResult(res)
	captureMediaTimelineRecordingStart("run-media", 100)

	checkpoint := farm.Artifact{Name: "round-002-frame-0000000310-gym.state"}
	artifact, err := drainMediaTimelineArtifact(farm.Spec{
		RunID: "run-media", Attempt: 1, Planner: "llm", Goal: "Earn the Boulder Badge.",
	}, "done", 500, []farm.Artifact{checkpoint})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Name != farm.MediaTimelineArtifactName {
		t.Fatalf("artifact name = %q", artifact.Name)
	}
	timeline, err := farm.DecodeMediaTimeline(artifact.Data)
	if err != nil {
		t.Fatal(err)
	}
	if timeline.SourceStartFrame != 100 || timeline.EndFrame != 400 {
		t.Fatalf("frame bounds = start %d end %d", timeline.SourceStartFrame, timeline.EndFrame)
	}
	if len(timeline.Snapshots) < 4 || timeline.Snapshots[0].Frame != 20 {
		t.Fatalf("snapshots = %#v", timeline.Snapshots)
	}

	for _, eventType := range []string{"objective_blocked", "failure_recovered", "gym_battle", "badge_acquired", "evolution", "checkpoint", "run_finished"} {
		if !timelineHasEvent(timeline, eventType) {
			t.Fatalf("missing %q event in %#v", eventType, timeline.Events)
		}
	}
	for _, event := range timeline.Events {
		if event.ID == "" {
			t.Fatalf("event without stable ID: %#v", event)
		}
	}
}

func TestDrainMediaTimelineArtifactWithoutResultIsOptional(t *testing.T) {
	resetMediaTimelineTelemetry()
	artifact, err := drainMediaTimelineArtifact(farm.Spec{RunID: "old-run"}, "done", 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Name != "" {
		t.Fatalf("unexpected artifact %#v", artifact)
	}
}

func TestCheckpointFrameParsing(t *testing.T) {
	frame, ok := checkpointFrameFromName("major-badge-02-round-014-frame-0000123456-post-gym.state")
	if !ok || frame != 123456 {
		t.Fatalf("frame = %d, %v", frame, ok)
	}
	round, ok := checkpointRoundFromName("major-badge-02-round-014-frame-0000123456-post-gym.state")
	if !ok || round != 14 {
		t.Fatalf("round = %d, %v", round, ok)
	}
}

func timelineHasEvent(timeline farm.MediaTimeline, eventType string) bool {
	for _, event := range timeline.Events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
