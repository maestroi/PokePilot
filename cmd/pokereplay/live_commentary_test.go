package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/media/commentator"
)

func commentaryWall(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/live" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"run": map[string]any{
			"run_id": "live", "status": "running", "frame": 2000, "goal": "earn the first badge",
			"attempts": 0, "map": 2, "x": 4, "y": 5, "activity": []map[string]any{
				{"source": "milestone", "kind": "badge", "frame": 1990, "attempt": 1, "summary": "Boulder Badge obtained"},
			},
		}})
	}))
}
func TestLiveCommentaryOperatorRoutesAndIsolation(t *testing.T) {
	wall := commentaryWall(t)
	defer wall.Close()
	replay := newReplayServer(wall.URL, "", "", nil)
	defer replay.stopLiveCommentaries()
	httpServer := httptest.NewServer(replay.handler())
	defer httpServer.Close()

	start := `{"enabled":true,"style":"documentary","verbosity":"sparse","cadence_seconds":60,"tts":false}`
	resp, err := http.Post(httpServer.URL+"/v1/runs/live/live/commentary/start", "application/json", strings.NewReader(start))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("start status=%d", resp.StatusCode)
	}
	deadline := time.Now().Add(2 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		res, err := http.Get(httpServer.URL + "/v1/runs/live/live/commentary/status")
		if err != nil {
			t.Fatal(err)
		}
		var status liveCommentaryStatus
		decodeErr := json.NewDecoder(res.Body).Decode(&status)
		_ = res.Body.Close()
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if status.Emitted > 0 {
			if len(status.Recent) != 1 || status.Recent[0].Text != "Boulder Badge obtained" {
				t.Fatalf("unsafe output: %+v", status)
			}
			found = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !found {
		t.Fatal("commentator did not observe semantic milestone")
	}
	// Commentary must not cause the frame/runner endpoint to block, and its
	// interaction is only with the wall read API, never planner/controller paths.
	started := time.Now()
	res, err := http.Get(httpServer.URL + "/v1/runs/live/live/status")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if time.Since(started) > time.Second {
		t.Fatal("commentary blocked live presentation")
	}
	res, err = http.Post(httpServer.URL+"/v1/runs/live/live/commentary/stop", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stop=%d", res.StatusCode)
	}
}
func TestLiveCommentaryInvalidSettingsFailClosed(t *testing.T) {
	wall := commentaryWall(t)
	defer wall.Close()
	replay := newReplayServer(wall.URL, "", "", nil)
	srv := httptest.NewServer(replay.handler())
	defer srv.Close()
	for _, input := range []string{
		`{"enabled":true,"style":"arbitrary","verbosity":"sparse","cadence_seconds":60}`,
		`{"enabled":true,"style":"calm","verbosity":"sparse","cadence_seconds":1}`,
		`{"enabled":true,"style":"calm","verbosity":"sparse","cadence_seconds":60,"unexpected":"secret"}`,
	} {
		res, err := http.Post(srv.URL+"/v1/runs/live/live/commentary/start", "application/json", strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("accepted invalid settings %s: %d", input, res.StatusCode)
		}
	}
}
func TestTTSFailureFallsBackToTextOnly(t *testing.T) {
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "private provider error", http.StatusServiceUnavailable)
	}))
	defer tts.Close()
	t.Setenv("POKEPILOT_LIVE_TTS_URL", tts.URL)
	t.Setenv("POKEPILOT_LIVE_TTS_MODEL", "test-speech")
	wall := commentaryWall(t)
	defer wall.Close()
	replay := newReplayServer(wall.URL, "", "", nil)
	engine, err := commentator.New(commentator.Settings{
		Enabled: true, Style: "calm", Verbosity: "sparse", CadenceSeconds: 60, TTS: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	session := &liveCommentarySession{
		server: replay, runID: "live", engine: engine,
		settings: engine.Settings, audio: make(chan []byte, 2),
	}
	run, err := replay.fetchLiveRun(context.Background(), "live")
	if err != nil {
		t.Fatal(err)
	}
	session.process(context.Background(), run)
	status := session.snapshot()
	if status.Emitted != 1 || len(status.Recent) != 1 || status.Recent[0].TTSState != "failed" {
		t.Fatalf("TTS failure leaked into commentary status %+v", status)
	}
	if len(session.audio) != 0 {
		t.Fatal("invalid TTS audio was enqueued")
	}
	if strings.Contains(status.LastError, "private provider error") {
		t.Fatal("provider secret exposed")
	}
}

type capturePCM struct {
	mu     sync.Mutex
	buf    []byte
	closed bool
}

func (c *capturePCM) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, io.ErrClosedPipe
	}
	c.buf = append(c.buf, p...)
	return len(p), nil
}
func (c *capturePCM) Close() error { c.mu.Lock(); c.closed = true; c.mu.Unlock(); return nil }
func TestCommentaryAudioPipeAndSilencePadding(t *testing.T) {
	cfg, err := normalizeRTMPBroadcastRequest(rtmpBroadcastRequest{
		Provider: "generic", Endpoint: "rtmp://relay.example.test/live", Audio: boolPtr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(rtmpFFmpegArgsCommentary(cfg), " ")
	if !strings.Contains(args, "pipe:3") || !strings.Contains(args, "-ar 24000") || strings.Contains(args, "anullsrc") {
		t.Fatalf("commentary FFmpeg missing PCM input: %s", args)
	}
	ctx, cancel := context.WithCancel(context.Background())
	queue := make(chan []byte, 1)
	queue <- []byte{9, 8, 7, 6}
	capture := &capturePCM{}
	done := make(chan struct{})
	go func() { feedCommentaryPCM(ctx, capture, queue); close(done) }()
	time.Sleep(240 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("PCM feed did not cancel")
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.buf) < commentaryPCMTickBytes {
		t.Fatalf("audio bytes=%d", len(capture.buf))
	}
	if capture.buf[0] != 9 || capture.buf[1] != 8 || capture.buf[2] != 7 || capture.buf[3] != 6 {
		t.Fatalf("audio wasn't delivered: %v", capture.buf[:4])
	}
	for _, v := range capture.buf[4:] {
		if v != 0 {
			t.Fatal("silence not padded")
		}
	}
}
func TestTerminalEventClassificationAndCommentary(t *testing.T) {
	event := liveRunActivity{Source: "system", Kind: "finished", Summary: "Run completed", Frame: 600, Attempt: 1}
	if typ := liveActivityEventType(event); typ != "run_finished" {
		t.Fatalf("terminal event type=%q", typ)
	}
	engine, err := commentator.New(commentator.Settings{Enabled: true, Style: "calm", Verbosity: "normal", CadenceSeconds: 60}, nil)
	if err != nil {
		t.Fatal(err)
	}
	replay := newReplayServer("http://unused.example", "", "", nil)
	session := &liveCommentarySession{server: replay, runID: "done", engine: engine, settings: engine.Settings, audio: make(chan []byte, 1)}
	session.process(context.Background(), liveRunView{
		RunID: "done", Status: "done", Frame: 600, Activity: []liveRunActivity{event},
	})
	got := session.snapshot()
	if got.Emitted != 1 || got.Recent[0].EventType != "run_finished" {
		t.Fatalf("terminal emission=%+v", got)
	}
}

func TestLiveCommentaryObjectKeyAvoidsPlainRunID(t *testing.T) {
	item := commentator.Emission{Attempt: 1, EventID: "evt", EmittedAt: time.Unix(123, 0)}
	a := liveCommentaryObjectKey("private-run-id", item)
	b := liveCommentaryObjectKey("private-run-id", item)
	if a != b || strings.Contains(a, "private-run-id") {
		t.Fatalf("invalid durable key: %s", a)
	}
	item.EventID = "different"
	if liveCommentaryObjectKey("private-run-id", item) == a {
		t.Fatal("distinct events wrote same artifact")
	}
}
