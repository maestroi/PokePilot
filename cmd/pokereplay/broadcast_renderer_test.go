package main

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestBroadcastPlanTracksTimelineTransitions(t *testing.T) {
	timeline := farm.MediaTimeline{
		Run:             farm.MediaRunSummary{RunID: "run-1", Goal: "become champion", Planner: "finish the league"},
		EndFrame:        600,
		FramesPerSecond: 60,
		Snapshots: []farm.MediaSnapshot{
			{
				Frame:     120,
				Objective: "reach Indigo Plateau",
				Location:  farm.MediaLocation{Name: "Route 23", X: 4, Y: 9},
				Player: &farm.Player{
					Badges: []string{"Boulder", "Cascade"},
					Party:  []farm.PartyMon{{Name: "VENUSAUR", Level: 42, HP: 99, MaxHP: 120}},
				},
				Planner: farm.MediaPlannerState{Intent: "prepare for the Elite Four"},
			},
			{
				Frame:     300,
				Objective: "defeat Lorelei",
				Location:  farm.MediaLocation{Place: "Indigo Plateau", X: 7, Y: 3},
				Player: &farm.Player{
					Badges: []string{"Boulder", "Cascade", "Thunder"},
					Party:  []farm.PartyMon{{Name: "VENUSAUR", Level: 45, HP: 110, MaxHP: 128}},
				},
			},
		},
	}.Normalized()

	plan := buildBroadcastPlan("run-1", 2, timeline)
	if len(plan.States) != 3 {
		t.Fatalf("states=%d, want initial + two transitions", len(plan.States))
	}
	if got := plan.States[1]; got.StartMS != 2000 || got.EndMS != 5000 || got.Objective != "reach Indigo Plateau" {
		t.Fatalf("first transition=%+v", got)
	}
	if got := plan.States[2]; got.StartMS != 5000 || got.EndMS != 0 || got.Location != "Indigo Plateau  (7,3)" {
		t.Fatalf("second transition=%+v", got)
	}
	if got := plan.States[1].Party; len(got) != 1 || !strings.Contains(got[0], "L42") || !strings.Contains(got[0], "99/120") {
		t.Fatalf("party=%v", got)
	}
}

func TestBroadcastPlanEventCardsHaveDeterministicTimingAndLanes(t *testing.T) {
	timeline := farm.MediaTimeline{
		Run:             farm.MediaRunSummary{RunID: "run-events"},
		EndFrame:        600,
		FramesPerSecond: 60,
		Events: []farm.MediaEvent{
			{Type: "badge_acquired", Frame: 60, Summary: "Boulder Badge", Evidence: "badge:boulder"},
			{Type: "checkpoint", Frame: 120, Summary: "Pewter checkpoint", Evidence: "checkpoint:pewter"},
			{Type: "blackout", Frame: 180, Summary: "Party wiped", Evidence: "battle:wipe"},
			{Type: "recovery", Frame: 360, Summary: "Recovered", Evidence: "recovery:center"},
		},
	}.Normalized()

	plan := buildBroadcastPlan("run-events", 1, timeline)
	if len(plan.Events) != 4 {
		t.Fatalf("events=%d", len(plan.Events))
	}
	if got := plan.Events[0]; got.StartMS != 1000 || got.EndMS != 5000 || got.Lane != 0 || got.Kind != "BADGE ACQUIRED" {
		t.Fatalf("event0=%+v", got)
	}
	if plan.Events[1].Lane != 1 || plan.Events[2].Lane != 2 {
		t.Fatalf("overlap lanes=%d,%d", plan.Events[1].Lane, plan.Events[2].Lane)
	}
	if plan.Events[3].Lane != 0 {
		t.Fatalf("reused lane=%d, want 0 after first card expired", plan.Events[3].Lane)
	}
}

func TestReplayCacheKeyVersionsBroadcastWithoutChangingRaw(t *testing.T) {
	recordings := []replayRecording{{
		Attempt: 1,
		Artifact: artifactRef{
			SHA256:    strings.Repeat("a", 64),
			ObjectKey: "runs/run-1/attempt-1/run.gbrun",
		},
	}}
	raw := replaySetCacheKey("run-1", recordings)
	if got := replaySetCacheKeyForMode("run-1", recordings, replayModeRaw, broadcastRendererVersion); got != raw {
		t.Fatalf("raw key=%q, want legacy %q", got, raw)
	}
	broadcast := replaySetCacheKeyForMode("run-1", recordings, replayModeBroadcast, broadcastRendererVersion)
	if broadcast == raw || !strings.Contains(broadcast, broadcastRendererVersion) {
		t.Fatalf("broadcast key=%q raw=%q", broadcast, raw)
	}
	if got := replaySetCacheKeyForMode("run-1", recordings, replayModeBroadcast, "broadcast-1280x720-v2"); got == broadcast {
		t.Fatalf("renderer version did not change cache key: %q", got)
	}
}

func TestParseReplayModeDefaultsToBroadcastAndKeepsRawFallback(t *testing.T) {
	if got, err := parseReplayMode(""); err != nil || got != replayModeBroadcast {
		t.Fatalf("default mode=%q err=%v", got, err)
	}
	if got, err := parseReplayMode("raw"); err != nil || got != replayModeRaw {
		t.Fatalf("raw mode=%q err=%v", got, err)
	}
	if _, err := parseReplayMode("unknown"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestFFmpegEnableBoundaries(t *testing.T) {
	if got := ffmpegEnable(2000, 5000); got != "between(t,2.000,5.000)" {
		t.Fatalf("bounded enable=%q", got)
	}
	if got := ffmpegEnable(5000, 0); got != "gte(t,5.000)" {
		t.Fatalf("open enable=%q", got)
	}
}
