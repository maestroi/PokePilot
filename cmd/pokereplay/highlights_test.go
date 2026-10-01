package main

import (
	"strings"
	"testing"

	mediahighlight "github.com/maestroi/pokepilot/media/highlight"
)

func testHighlightPlan(hash string) mediahighlight.Plan {
	return mediahighlight.Plan{
		Version:          mediahighlight.PlanVersion,
		PolicyVersion:    mediahighlight.PolicyVersion,
		TargetDurationMS: 480000,
		DurationMS:       35000,
		Hash:             hash,
		Windows: []mediahighlight.Window{{
			Attempt: 1, Index: 0, StartFrame: 100, EndFrame: 200,
			StartMS: 1000, EndMS: 2000, Priority: 100,
			Events: []mediahighlight.EventRef{{ID: "evt-1", Type: "badge_acquired", Frame: 150, Priority: 100}},
		}},
	}
}

func testHighlightRecordings() []replayRecording {
	return []replayRecording{{
		Attempt:  1,
		Artifact: artifactRef{SHA256: strings.Repeat("a", 64), ObjectKey: "runs/run-1/attempt-1/run.gbrun"},
		Timeline: artifactRef{SHA256: strings.Repeat("b", 64), ObjectKey: "runs/run-1/attempt-1/media-timeline.json"},
	}}
}

func TestHighlightIdentityChangesWithPlanAndTimeline(t *testing.T) {
	s := newReplayServer("http://wall.invalid", "", "", nil)
	recordings := testHighlightRecordings()
	plan := testHighlightPlan(strings.Repeat("c", 64))

	videoA, manifestA := s.highlightKeys("run-1", recordings, plan)
	videoAgain, manifestAgain := s.highlightKeys("run-1", recordings, plan)
	if videoA != videoAgain || manifestA != manifestAgain {
		t.Fatalf("same highlight inputs changed identity: %q/%q vs %q/%q", videoA, manifestA, videoAgain, manifestAgain)
	}

	changedPlan := plan
	changedPlan.Hash = strings.Repeat("d", 64)
	videoB, _ := s.highlightKeys("run-1", recordings, changedPlan)
	if videoB == videoA {
		t.Fatalf("changed highlight plan reused artifact identity %q", videoA)
	}

	changedTimeline := testHighlightRecordings()
	changedTimeline[0].Timeline.SHA256 = strings.Repeat("e", 64)
	videoC, _ := s.highlightKeys("run-1", changedTimeline, plan)
	if videoC == videoA {
		t.Fatalf("changed semantic timeline reused artifact identity %q", videoA)
	}
}

func TestHighlightClipIdentityIncludesWindow(t *testing.T) {
	key := "runs/run-1/highlights/highlight-v1-abc.mp4"
	a := highlightClipCacheKey(key, mediahighlight.Window{Index: 0, Attempt: 1, StartFrame: 100, EndFrame: 200})
	b := highlightClipCacheKey(key, mediahighlight.Window{Index: 0, Attempt: 1, StartFrame: 101, EndFrame: 200})
	if a == b {
		t.Fatalf("window bounds did not affect clip key: %q", a)
	}
	if !strings.Contains(a, "a1-f100-200.mp4") {
		t.Fatalf("clip key does not describe source window: %q", a)
	}
}

func TestHighlightPolicyEnvironmentOverridesBudgetAndGap(t *testing.T) {
	t.Setenv(highlightPolicyJSONEnv, "")
	t.Setenv(highlightTargetSecondsEnv, "90")
	t.Setenv(highlightMergeGapSecondsEnv, "7")
	policy := highlightPolicyFromEnv()
	if policy.TargetDurationMS != 90000 {
		t.Fatalf("target=%d want 90000", policy.TargetDurationMS)
	}
	if policy.MergeGapMS != 7000 {
		t.Fatalf("merge gap=%d want 7000", policy.MergeGapMS)
	}
	if len(policy.Rules) == 0 {
		t.Fatal("environment override discarded built-in event rules")
	}
}

func TestHighlightProfileUsesPlanAsEditIdentity(t *testing.T) {
	s := newReplayServer("http://wall.invalid", "", "", nil)
	plan := testHighlightPlan(strings.Repeat("f", 64))
	profile := s.highlightProfile(plan)
	if profile.EditPlan == nil {
		t.Fatal("highlight profile omitted edit plan identity")
	}
	if profile.EditPlan.SHA256 != plan.Hash {
		t.Fatalf("edit plan hash=%q want %q", profile.EditPlan.SHA256, plan.Hash)
	}
	if string(profile.Name) != "highlight" {
		t.Fatalf("profile name=%q want highlight", profile.Name)
	}
}
