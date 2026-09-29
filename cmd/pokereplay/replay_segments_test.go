package main

import (
	"os"
	"strings"
	"testing"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

func TestPlanReplayVideoSegmentsNoGapsOrDuplicates(t *testing.T) {
	recording := &gomeboy.Recording{StartFrame: 100, EndFrame: 109}
	got := planReplayVideoSegments(2, recording, 4)
	if len(got) != 3 {
		t.Fatalf("segments=%d, want 3: %+v", len(got), got)
	}
	want := [][2]uint64{{0, 3}, {4, 7}, {8, 9}}
	for i, segment := range got {
		if segment.Attempt != 2 || segment.Index != i {
			t.Fatalf("segment %d identity=%+v", i, segment)
		}
		if segment.StartFrame != want[i][0] || segment.EndFrame != want[i][1] {
			t.Fatalf("segment %d range=%d..%d, want %d..%d", i, segment.StartFrame, segment.EndFrame, want[i][0], want[i][1])
		}
		if i > 0 && got[i-1].EndFrame+1 != segment.StartFrame {
			t.Fatalf("gap/overlap between %+v and %+v", got[i-1], segment)
		}
	}
}

func TestPlanReplayVideoSegmentsIncludesRestoredInitialFrame(t *testing.T) {
	recording := &gomeboy.Recording{StartFrame: 77, EndFrame: 77}
	got := planReplayVideoSegments(1, recording, 100)
	if len(got) != 1 || got[0].StartFrame != 0 || got[0].EndFrame != 0 {
		t.Fatalf("single-frame recording plan=%+v", got)
	}
}

func TestReplayVideoSegmentCacheKeyIncludesRangeAndPlan(t *testing.T) {
	s := newReplayServer("http://wall.invalid", "", "", nil)
	recording := replayRecording{
		Attempt: 3,
		Artifact: artifactRef{
			SHA256:    strings.Repeat("ab", 32),
			ObjectKey: "runs/run-1/attempt-3/run.gbrun",
		},
	}
	first := replayVideoSegment{Attempt: 3, Index: 0, StartFrame: 0, EndFrame: 99}
	second := replayVideoSegment{Attempt: 3, Index: 1, StartFrame: 100, EndFrame: 199}
	a := s.replayVideoSegmentCacheKey("run-1", recording, replayModeRaw, 100, first)
	b := s.replayVideoSegmentCacheKey("run-1", recording, replayModeRaw, 100, second)
	if a == b {
		t.Fatalf("different ranges share cache key %q", a)
	}
	if !strings.Contains(a, "segments-v1-f100") || !strings.Contains(a, "00000-f0-99.mp4") {
		t.Fatalf("unexpected segment cache key %q", a)
	}
}

func TestReplaySegmentFramesConfig(t *testing.T) {
	old, had := os.LookupEnv(replaySegmentSecondsEnv)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(replaySegmentSecondsEnv, old)
		} else {
			_ = os.Unsetenv(replaySegmentSecondsEnv)
		}
	})
	if err := os.Setenv(replaySegmentSecondsEnv, "10"); err != nil {
		t.Fatal(err)
	}
	got := replaySegmentFrames()
	if got < 590 || got > 610 {
		t.Fatalf("10-second segment frames=%d, want about 597", got)
	}
}
