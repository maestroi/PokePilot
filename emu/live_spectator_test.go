package emu

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLiveFrameQueueCompactsOverflowAcrossTimeline(t *testing.T) {
	q := newLiveFrameQueue(4)
	for _, frame := range []uint64{3, 6, 9, 12, 15, 18, 21} {
		q.push(frame, []byte{byte(frame)})
	}

	q.mu.Lock()
	got := make([]uint64, len(q.frames))
	for i, frame := range q.frames {
		got[i] = frame.frame
	}
	q.mu.Unlock()

	want := []uint64{12, 18, 21}
	if len(got) != len(want) {
		t.Fatalf("compacted frames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("compacted frames = %v, want %v", got, want)
		}
	}

	latest, ok := q.latest()
	if !ok || latest.frame != 21 {
		t.Fatalf("latest after compaction = (%d, %t), want frame 21", latest.frame, ok)
	}
}

func TestLiveFrameQueuePlaybackAcceleratesBacklog(t *testing.T) {
	q := newLiveFrameQueue(20)
	for frame := uint64(3); frame <= 36; frame += 3 {
		q.push(frame, []byte{byte(frame)})
	}

	for _, want := range []uint64{12, 18, 24, 27} {
		got, ok := q.nextPlayback()
		if !ok {
			t.Fatalf("nextPlayback() missing frame %d", want)
		}
		if got.frame != want {
			t.Fatalf("nextPlayback() frame = %d, want %d", got.frame, want)
		}
	}

	// Once the backlog is small, playback returns to one sampled frame per
	// browser read instead of staying in fast-forward forever.
	got, ok := q.nextPlayback()
	if !ok || got.frame != 30 {
		t.Fatalf("near-live nextPlayback() = (%d, %t), want frame 30", got.frame, ok)
	}
}

func TestPlaybackAdvanceUsesBacklogBands(t *testing.T) {
	for _, tc := range []struct {
		depth int
		want  int
	}{
		{1, 1},
		{5, 1},
		{6, 2},
		{11, 2},
		{12, 4},
		{100, 4},
	} {
		if got := playbackAdvance(tc.depth); got != tc.want {
			t.Fatalf("playbackAdvance(%d) = %d, want %d", tc.depth, got, tc.want)
		}
	}
}

func TestLiveFrameQueueResetsOnFrameRollback(t *testing.T) {
	q := newLiveFrameQueue(4)
	q.push(300, []byte("old-a"))
	q.push(303, []byte("old-b"))
	q.push(12, []byte("new-run"))

	got, ok := q.nextPlayback()
	if !ok {
		t.Fatal("nextPlayback() missing first frame after rollback")
	}
	if got.frame != 12 || string(got.png) != "new-run" {
		t.Fatalf("nextPlayback() after rollback = (%d, %q), want (12, %q)", got.frame, got.png, "new-run")
	}
}

func TestLiveFrameQueueExplicitResetStartsNewEpochWhenFrameIncreases(t *testing.T) {
	q := newLiveFrameQueue(8)
	q.push(300, []byte("boot-a"))
	q.push(303, []byte("boot-b"))

	// Durable resume checkpoints commonly have a larger frame count than the
	// worker's one-time boot sequence. The epoch must therefore be explicit,
	// not inferred only from a decreasing frame number.
	q.reset(900000, []byte("restored"))

	got, ok := q.nextPlayback()
	if !ok {
		t.Fatal("nextPlayback() missing restored frame after explicit reset")
	}
	if got.frame != 900000 || string(got.png) != "restored" {
		t.Fatalf("nextPlayback() after explicit reset = (%d, %q), want (900000, %q)", got.frame, got.png, "restored")
	}
	if _, ok := q.nextPlayback(); !ok {
		t.Fatal("nextPlayback() should retain the restored frame once queue is drained")
	}
}

func TestLiveSpectatorUsesNativeTimeStride(t *testing.T) {
	if got := newLiveSpectator(1).captureEvery; got != 3 {
		t.Fatalf("capture-every 1 stride = %d, want 3", got)
	}
	if got := newLiveSpectator(4).captureEvery; got != 4 {
		t.Fatalf("capture-every 4 stride = %d, want 4", got)
	}
}

func TestLiveSpectatorHandlerServesFrameMetadata(t *testing.T) {
	s := newLiveSpectator(1)
	s.queue.push(42, []byte("png-bytes"))

	req := httptest.NewRequest(http.MethodGet, "/frame.png", nil)
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if got := res.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := res.Header().Get("X-PokePilot-Frame"); got != "42" {
		t.Fatalf("X-PokePilot-Frame = %q, want 42", got)
	}
	body, err := io.ReadAll(res.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "png-bytes" {
		t.Fatalf("body = %q, want png-bytes", body)
	}
}

func TestLiveSpectatorHandlerWaitsForFirstFrame(t *testing.T) {
	s := newLiveSpectator(1)
	req := httptest.NewRequest(http.MethodGet, "/frame.png", nil)
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusServiceUnavailable)
	}
}
