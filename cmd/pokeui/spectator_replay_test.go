package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSpectatorReplayIsReadOnlyAndSanitized(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == "/v1/dashboard" {
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"now":123,"runs":[{"run_id":"run-done","status":"done","goal":"Earn the Boulder Badge.","ended_at":120}]}`)) //nolint:errcheck
			return
		}
		http.NotFound(res, req)
	}))
	t.Cleanup(wall.Close)

	var replayCalls atomic.Int32
	replay := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		replayCalls.Add(1)
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/runs/run-done/replay/status":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"run_id":"run-done","state":"ready","object_key":"private/runs/run-done/replay.mp4","size":6,"error":"private storage detail"}`)) //nolint:errcheck
		case req.Method == http.MethodGet && req.URL.Path == "/v1/runs/run-done/replay/video":
			if got := req.Header.Get("Range"); got != "bytes=1-3" {
				t.Errorf("Range = %q, want bytes=1-3", got)
			}
			res.Header().Set("Content-Type", "video/mp4")
			res.Header().Set("Accept-Ranges", "bytes")
			res.Header().Set("Content-Range", "bytes 1-3/6")
			res.WriteHeader(http.StatusPartialContent)
			res.Write([]byte("234")) //nolint:errcheck
		default:
			t.Errorf("unexpected replay request: %s %s", req.Method, req.URL.Path)
			http.NotFound(res, req)
		}
	}))
	t.Cleanup(replay.Close)

	ui := httptest.NewServer(spectatorHandlerWithReplay(wall.URL, replay.URL))
	t.Cleanup(ui.Close)

	res, err := http.Get(ui.URL + "/v1/watch/runs/run-done/replay/status")
	if err != nil {
		t.Fatalf("GET public replay status: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", res.StatusCode, body)
	}
	for _, want := range []string{`"run_id":"run-done"`, `"state":"ready"`, `"size":6`} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("public status missing %s: %s", want, body)
		}
	}
	for _, secret := range []string{"object_key", "private/runs", "private storage detail", `"error"`} {
		if bytes.Contains(body, []byte(secret)) {
			t.Errorf("public replay status leaked %q: %s", secret, body)
		}
	}

	req, err := http.NewRequest(http.MethodGet, ui.URL+"/v1/watch/runs/run-done/replay/video", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=1-3")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET public replay video: %v", err)
	}
	video, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("video status = %d, want 206", res.StatusCode)
	}
	if string(video) != "234" {
		t.Fatalf("video body = %q, want 234", video)
	}
	if got := res.Header.Get("Content-Range"); got != "bytes 1-3/6" {
		t.Errorf("Content-Range = %q", got)
	}

	beforeBlocked := replayCalls.Load()
	for _, probe := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/watch/runs/run-done/replay/render"},
		{http.MethodGet, "/v1/watch/runs/run-done/artifacts"},
		{http.MethodGet, "/v1/watch/runs/run-done/debug"},
	} {
		req, err := http.NewRequest(probe.method, ui.URL+probe.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", probe.method, probe.path, err)
		}
		io.Copy(io.Discard, res.Body) //nolint:errcheck
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", probe.method, probe.path, res.StatusCode)
		}
	}
	if got := replayCalls.Load(); got != beforeBlocked {
		t.Fatalf("blocked public replay routes reached replay service: %d -> %d", beforeBlocked, got)
	}

	src := string(watchJS)
	for _, want := range []string{"/v1/watch/runs/", "/replay/status", "/replay/video", "playback-rate", "completedSummary"} {
		if !strings.Contains(src, want) {
			t.Errorf("watch.js missing spectator replay behavior %q", want)
		}
	}
	for _, forbidden := range []string{"/replay/render", "/artifacts", "/debug"} {
		if strings.Contains(src, forbidden) {
			t.Errorf("watch.js unexpectedly exposes %q", forbidden)
		}
	}
}

func TestSpectatorSummaryCountsWholeFarmBeforeHistoryTrim(t *testing.T) {
	runs := []spectatorRun{
		{RunID: "running", Status: "running"},
		{RunID: "leased", Status: "leased"},
		{RunID: "queued", Status: "queued"},
		{RunID: "done-1", Status: "done"},
		{RunID: "done-2", Status: "done"},
	}
	got := summarizeSpectatorRuns(runs)
	if got.Live != 2 || got.Queued != 1 || got.Completed != 2 {
		t.Fatalf("summary = %+v, want live=2 queued=1 completed=2", got)
	}
}
