package emu

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLiveFrameQueuePreservesSmoothSegmentThenResyncs(t *testing.T) {
	q := newLiveFrameQueue(3)
	q.push(3, []byte("three"))
	q.push(6, []byte("six"))
	q.push(9, []byte("nine"))

	// Overflow does not evict the smooth segment already waiting for the
	// spectator. It collapses to the newest pending frame instead.
	q.push(12, []byte("twelve"))
	q.push(15, []byte("fifteen"))

	got, ok := q.next()
	if !ok || got.frame != 3 {
		t.Fatalf("first next() = (%d, %t), want frame 3", got.frame, ok)
	}

	// Even though one queue slot is now free, keep collapsing overflow until
	// the original segment drains so MAX-speed execution cannot interleave
	// large jumps between otherwise adjacent display frames.
	q.push(18, []byte("eighteen"))

	for _, want := range []struct {
		frame uint64
		body  string
	}{
		{6, "six"},
		{9, "nine"},
		{18, "eighteen"},
	} {
		got, ok = q.next()
		if !ok {
			t.Fatalf("next() missing frame %d", want.frame)
		}
		if got.frame != want.frame || string(got.png) != want.body {
			t.Fatalf("next() = (%d, %q), want (%d, %q)", got.frame, got.png, want.frame, want.body)
		}
	}

	got, ok = q.next()
	if !ok {
		t.Fatal("next() should hold the last displayed frame when the producer pauses")
	}
	if got.frame != 18 || string(got.png) != "eighteen" {
		t.Fatalf("held frame = (%d, %q), want (18, %q)", got.frame, got.png, "eighteen")
	}
}

func TestLiveFrameQueueLatestUsesOverflowResyncFrame(t *testing.T) {
	q := newLiveFrameQueue(2)
	q.push(3, []byte("three"))
	q.push(6, []byte("six"))
	q.push(9, []byte("nine"))
	q.push(12, []byte("twelve"))

	got, ok := q.latest()
	if !ok {
		t.Fatal("latest() missing overflow frame")
	}
	if got.frame != 12 || string(got.png) != "twelve" {
		t.Fatalf("latest() = (%d, %q), want (12, %q)", got.frame, got.png, "twelve")
	}
}

func TestLiveFrameQueueResetsOnFrameRollback(t *testing.T) {
	q := newLiveFrameQueue(4)
	q.push(300, []byte("old-a"))
	q.push(303, []byte("old-b"))
	q.push(12, []byte("new-run"))

	got, ok := q.next()
	if !ok {
		t.Fatal("next() missing first frame after rollback")
	}
	if got.frame != 12 || string(got.png) != "new-run" {
		t.Fatalf("next() after rollback = (%d, %q), want (12, %q)", got.frame, got.png, "new-run")
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
