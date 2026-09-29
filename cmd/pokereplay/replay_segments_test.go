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

func TestWindowBroadcastPlanPreservesBoundaryStateAndEventRemainder(t *testing.T) {
	plan := broadcastPlan{
		States: []broadcastState{
			{StartMS: 0, EndMS: 5000, Elapsed: "00:00", Objective: "first"},
			{StartMS: 5000, EndMS: 10000, Elapsed: "00:05", Objective: "second"},
		},
		Events: []broadcastEventCard{
			{StartMS: 3500, EndMS: 7500, Lane: 1, Kind: "BADGE", Summary: "crosses"},
			{StartMS: 8000, EndMS: 9000, Lane: 0, Kind: "ITEM", Summary: "inside"},
		},
	}
	got := windowBroadcastPlan(plan, 4000, 8000)
	if len(got.States) != 2 {
		t.Fatalf("states=%+v", got.States)
	}
	if got.States[0].StartMS != 0 || got.States[0].EndMS != 1000 || got.States[0].Objective != "first" {
		t.Fatalf("initial boundary state=%+v", got.States[0])
	}
	if got.States[1].StartMS != 1000 || got.States[1].EndMS != 4000 || got.States[1].Objective != "second" {
		t.Fatalf("later state=%+v", got.States[1])
	}
	if len(got.Events) != 1 {
		t.Fatalf("events=%+v, want only crossing event", got.Events)
	}
	if got.Events[0].StartMS != 0 || got.Events[0].EndMS != 3500 || got.Events[0].Lane != 1 {
		t.Fatalf("crossing event window=%+v", got.Events[0])
	}
}

func TestProbeReplaySegmentCacheRejectsCorruptObject(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available")
	}
	body := []byte("not an mp4")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	}))
	defer server.Close()

	store, err := artifactstore.NewS3(artifactstore.S3Config{
		Endpoint: server.URL, Bucket: "pokepilot", AccessKey: "test", SecretKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := newReplayServer("", "", "", store)
	attempts := []preparedReplayAttempt{{
		Segments: []replayVideoSegment{{
			Attempt: 1, Index: 0, StartFrame: 0, EndFrame: 59, CacheKey: "runs/run-1/segments/bad.mp4",
		}},
	}}
	ready, err := s.probeReplaySegmentCache(context.Background(), attempts)
	if err != nil {
		t.Fatal(err)
	}
	if ready != 0 || attempts[0].Segments[0].Cached {
		t.Fatalf("corrupt cache marked ready: ready=%d segment=%+v", ready, attempts[0].Segments[0])
	}
}

func TestConcatReplaySegmentURLsStreamsRemoteInputs(t *testing.T) {
	for _, binary := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skip(binary + " not available")
		}
	}
	dir := t.TempDir()
	makeClip := func(name, color string) []byte {
		t.Helper()
		filename := filepath.Join(dir, name)
		cmd := exec.Command("ffmpeg",
			"-hide_banner", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", "color=c="+color+":s=160x144:r=10",
			"-t", "0.5",
			"-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart",
			filename,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("make clip %s: %v: %s", name, err, out)
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	clips := map[string][]byte{
		"/a.mp4": makeClip("a.mp4", "black"),
		"/b.mp4": makeClip("b.mp4", "white"),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := clips[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, r.URL.Path, time.Time{}, bytes.NewReader(data))
	}))
	defer server.Close()

	destination := filepath.Join(dir, "joined.mp4")
	if err := concatReplaySegmentURLs(context.Background(), dir, []string{
		server.URL + "/a.mp4",
		server.URL + "/b.mp4",
	}, destination); err != nil {
		t.Fatal(err)
	}
	if err := probeReplayVideo(context.Background(), destination, time.Second); err != nil {
		t.Fatalf("joined replay invalid: %v", err)
	}
}
