package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func testLivePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 160, 144))
	for y := 0; y < 144; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 64, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRenderLiveBroadcastFrameUsesBroadcastLayout(t *testing.T) {
	timeline := farm.MediaTimeline{
		Version:          farm.MediaTimelineVersion,
		Run:              farm.MediaRunSummary{RunID: "run-live", Goal: "beat Brock", Planner: "scripted"},
		Attempt:          1,
		EndFrame:         600,
		FramesPerSecond:  farm.GameBoyFramesPerSecond,
		Snapshots: []farm.MediaSnapshot{{
			Frame:     600,
			Objective: "battle Brock",
			Location:  farm.MediaLocation{Name: "Pewter Gym", X: 5, Y: 7},
			Player: &farm.Player{Badges: []string{"Boulder"}, Party: []farm.PartyMon{{
				Name: "SQUIRTLE", Level: 12, HP: 25, MaxHP: 30,
			}}},
		}},
	}.Normalized()

	data, err := renderLiveBroadcastFrame(testLivePNG(t), "run-live", 1, timeline)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode live jpeg: %v", err)
	}
	if got := img.Bounds().Size(); got.X != broadcastWidth || got.Y != broadcastHeight {
		t.Fatalf("live frame size=%v want=%dx%d", got, broadcastWidth, broadcastHeight)
	}
}

func TestLiveBroadcastSessionDropsStaleFramesForSlowConsumer(t *testing.T) {
	session := &liveBroadcastSession{
		status: liveBroadcastStatus{RunID: "slow", State: "starting", TargetFPS: liveBroadcastFPS},
		subscribers: make(map[uint64]chan liveEncodedFrame),
	}
	frames, unsubscribe := session.subscribe()
	defer unsubscribe()

	for frame := uint64(1); frame <= 10; frame++ {
		session.publish(liveEncodedFrame{frame: frame, jpeg: []byte{byte(frame)}})
	}

	var got []uint64
	for {
		select {
		case frame := <-frames:
			got = append(got, frame.frame)
		default:
			if len(got) == 0 || got[len(got)-1] != 10 {
				t.Fatalf("queued frames=%v, want newest frame 10", got)
			}
			status := session.snapshot()
			if status.State != "lagging" || status.DroppedFrames == 0 {
				t.Fatalf("status=%+v, want lagging with dropped frames", status)
			}
			if len(got) > liveBroadcastSubscriberQueue {
				t.Fatalf("queue grew beyond bound: %v", got)
			}
			return
		}
	}
}

func TestLiveBroadcastStreamReconnectsAndEndsCleanly(t *testing.T) {
	pngData := testLivePNG(t)
	var mu sync.Mutex
	runReads := 0
	frameReads := 0
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/run-live":
			mu.Lock()
			runReads++
			read := runReads
			mu.Unlock()
			status := "running"
			if read >= 4 {
				status = "done"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"run": map[string]any{
				"run_id": "run-live", "status": status, "goal": "beat Brock", "planner": "scripted",
				"frame": uint64(100 + read), "map": 2, "x": 5, "y": 7, "attempts": 0,
				"player": map[string]any{"badges": []string{"Boulder"}, "party": []map[string]any{{"name": "SQUIRTLE", "level": 12, "hp": 25, "max_hp": 30}}},
				"activity": []map[string]any{{"source": "milestone", "kind": "badge", "frame": 100, "attempt": 1, "summary": "Boulder Badge obtained"}},
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/frame":
			if r.URL.Query().Get("latest") != "1" {
				t.Errorf("live sidecar did not request point-in-time frame: %s", r.URL.RawQuery)
			}
			mu.Lock()
			frameReads++
			read := frameReads
			mu.Unlock()
			if read == 1 {
				http.Error(w, "temporary frame gap", http.StatusBadGateway)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer wall.Close()

	replay := newReplayServer(wall.URL, "", "", nil)
	srv := httptest.NewServer(replay.handler())
	defer func() {
		replay.stopLiveSessions()
		srv.Close()
	}()

	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get(srv.URL + "/v1/runs/run-live/live/stream.mjpeg")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stream status=%d", res.StatusCode)
	}
	if got := res.Header.Get("X-PokePilot-Live-FPS"); got != "20" {
		t.Fatalf("live fps header=%q", got)
	}

	reader := multipart.NewReader(res.Body, liveBroadcastBoundary)
	part, err := reader.NextPart()
	if err != nil {
		t.Fatalf("first live part: %v", err)
	}
	frameBytes, err := io.ReadAll(part)
	if err != nil {
		t.Fatal(err)
	}
	part.Close()
	img, err := jpeg.Decode(bytes.NewReader(frameBytes))
	if err != nil {
		t.Fatalf("first live frame is not jpeg: %v", err)
	}
	if got := img.Bounds().Size(); got.X != broadcastWidth || got.Y != broadcastHeight {
		t.Fatalf("stream frame size=%v", got)
	}

	// The fake wall ends the run on the fourth state sample. Multipart parsing
	// must then observe a clean end rather than hanging on a dead producer.
	for {
		part, err = reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("live end: %v", err)
		}
		_, _ = io.Copy(io.Discard, part)
		part.Close()
	}

	session := replay.liveSessionIfPresent("run-live")
	if session == nil {
		t.Fatal("live session disappeared before status could be inspected")
	}
	status := session.snapshot()
	if status.State != "ended" || status.Reconnects == 0 || status.Frame == 0 {
		t.Fatalf("final live status=%+v, want ended after a reconnect", status)
	}
	mu.Lock()
	gotFrameReads := frameReads
	mu.Unlock()
	if gotFrameReads < 2 {
		t.Fatalf("frame reads=%d, want failed read plus successful reconnect", gotFrameReads)
	}
}

func TestLiveStatusDoesNotStartProducer(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/run-idle" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"run":{"run_id":"run-idle","status":"running","frame":77}}`)
	}))
	defer wall.Close()

	replay := newReplayServer(wall.URL, "", "", nil)
	srv := httptest.NewServer(replay.handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/v1/runs/run-idle/live/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var status liveBroadcastStatus
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.State != "idle" || status.Frame != 77 {
		t.Fatalf("status=%+v", status)
	}
	if replay.liveSessionIfPresent("run-idle") != nil {
		t.Fatal("status probe started a live media producer")
	}
}
