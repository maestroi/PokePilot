package main

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
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

func TestBroadcastOverlayTimelineKeepsEventBoundaries(t *testing.T) {
	dir := t.TempDir()
	plan := broadcastPlan{
		States: []broadcastState{
			{StartMS: 0, Objective: "first"},
			{StartMS: 2000, Objective: "second"},
		},
		Events: []broadcastEventCard{{StartMS: 1000, EndMS: 5000, Lane: 0, Kind: "BADGE"}},
	}
	playlist, err := writeBroadcastOverlayTimeline(dir, plan)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(playlist)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "duration "); got != 4 {
		t.Fatalf("overlay transitions=%d, want 0, 1, 2, and 5 seconds:\n%s", got, data)
	}
	for i, wantCard := range []bool{false, true, true, false} {
		file, err := os.Open(filepath.Join(dir, "overlay-000"+string(rune('0'+i))+".png"))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(file)
		_ = file.Close()
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, alpha := img.At(44, 480).RGBA()
		if (alpha != 0) != wantCard {
			t.Fatalf("overlay %d event card alpha=%d, want card=%v", i, alpha, wantCard)
		}
	}
}

func TestBroadcastCompositorEncodesTimelineWithSparseOverlay(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.mp4")
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=blue:s=160x144:r=10", "-t", "6",
		"-c:v", "mpeg4", raw).CombinedOutput()
	if err != nil {
		t.Fatalf("prepare raw video: %v\n%s", err, out)
	}
	video := filepath.Join(dir, "broadcast.mp4")
	timeline := farm.MediaTimeline{
		Run:             farm.MediaRunSummary{RunID: "run-sparse"},
		FramesPerSecond: 60,
		EndFrame:        360,
		Snapshots:       []farm.MediaSnapshot{{Frame: 120, Objective: "second"}},
		Events:          []farm.MediaEvent{{Frame: 60, Type: "badge_acquired", Summary: "Badge"}},
	}.Normalized()
	compositor := &ffmpegBroadcastCompositor{binary: ffmpeg}
	if err := compositor.Compose(context.Background(), broadcastScene{
		RunID: "run-sparse", Attempt: 1, RawVideo: raw, Destination: video, Timeline: timeline,
	}); err != nil {
		t.Fatal(err)
	}
	before := broadcastPixel(t, ffmpeg, video, "0.5")
	during := broadcastPixel(t, ffmpeg, video, "2.0")
	after := broadcastPixel(t, ffmpeg, video, "5.5")
	if before == during || during == after {
		t.Fatalf("event card did not appear and expire: before=%v during=%v after=%v", before, during, after)
	}
}

func TestBroadcastCompositorInputCountDoesNotGrowWithTelemetry(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "capture-ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE_ARGS\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(dir, "args")
	t.Setenv("CAPTURE_ARGS", capture)
	timeline := farm.MediaTimeline{
		Run:             farm.MediaRunSummary{RunID: "run-many-cards"},
		FramesPerSecond: 60,
		EndFrame:        60000,
	}
	for i := 0; i < 96; i++ {
		timeline.Snapshots = append(timeline.Snapshots, farm.MediaSnapshot{Frame: uint64(i * 600)})
	}
	for i := 0; i < 157; i++ {
		timeline.Events = append(timeline.Events, farm.MediaEvent{Frame: uint64(i * 300), Type: "checkpoint"})
	}
	compositor := &ffmpegBroadcastCompositor{binary: ffmpeg}
	if err := compositor.Compose(context.Background(), broadcastScene{
		RunID: "run-many-cards", Attempt: 1, RawVideo: filepath.Join(dir, "raw.mp4"),
		Destination: filepath.Join(dir, "broadcast.mp4"), Timeline: timeline.Normalized(),
	}); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count("\n"+string(args), "\n-i\n"); got != 2 {
		t.Fatalf("96 snapshots and 157 events yielded %d FFmpeg inputs, want raw video plus one overlay", got)
	}
}

func broadcastPixel(t *testing.T, ffmpeg, video, at string) [3]uint8 {
	t.Helper()
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-ss", at,
		"-i", video, "-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(44, 480).RGBA()
	return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
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
