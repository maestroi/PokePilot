package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSpectatorOnlyPublishesNoteworthyReadyReplays(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == "/v1/dashboard" {
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{
				"now":123,
				"runs":[
					{"run_id":"run-live","status":"running","goal":"Beat the Elite Four and Champion.","queued_at":100},
					{"run_id":"run-goal","status":"done","goal":"Earn the Boulder Badge.","ended_at":110,"stats":{"goal_complete":true,"goal_summary":"Boulder Badge earned"}},
					{"run_id":"run-analyzed","status":"done","goal":"Reach Mt. Moon.","ended_at":111,"reason":"stagnation","issue":{"issue_number":42,"issue_url":"private"}},
					{"run_id":"run-boring","status":"done","goal":"Earn the Boulder Badge.","ended_at":112,"reason":"cancelled"},
					{"run_id":"run-no-video","status":"done","goal":"Earn the Boulder Badge.","ended_at":113,"stats":{"goal_complete":true}}
				]
		}`)) //nolint:errcheck
			return
		}
		http.NotFound(res, req)
	}))
	t.Cleanup(wall.Close)

	var replayCalls atomic.Int32
	var boringStatusCalls atomic.Int32
	replay := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		replayCalls.Add(1)
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/runs/run-goal/replay/status":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"run_id":"run-goal","state":"ready","object_key":"private/runs/run-goal/replay.mp4","size":6,"error":"private storage detail"}`)) //nolint:errcheck
		case req.Method == http.MethodGet && req.URL.Path == "/v1/runs/run-analyzed/replay/status":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"run_id":"run-analyzed","state":"ready","object_key":"private/runs/run-analyzed/replay.mp4","size":8}`)) //nolint:errcheck
		case req.Method == http.MethodGet && req.URL.Path == "/v1/runs/run-no-video/replay/status":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"run_id":"run-no-video","state":"missing"}`)) //nolint:errcheck
		case req.Method == http.MethodGet && req.URL.Path == "/v1/runs/run-boring/replay/status":
			boringStatusCalls.Add(1)
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"run_id":"run-boring","state":"ready","size":6}`)) //nolint:errcheck
		case req.Method == http.MethodGet && req.URL.Path == "/v1/runs/run-goal/replay/video":
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

	res, err := http.Get(ui.URL + "/v1/watch")
	if err != nil {
		t.Fatalf("GET public snapshot: %v", err)
	}
	watchBody, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/watch = %d: %s", res.StatusCode, watchBody)
	}
	for _, want := range []string{"run-live", "run-goal", "run-analyzed", `"replay_ready":true`, `"highlight":"goal complete"`, `"highlight":"analyzed"`} {
		if !bytes.Contains(watchBody, []byte(want)) {
			t.Errorf("public snapshot missing %q: %s", want, watchBody)
		}
	}
	for _, hidden := range []string{"run-boring", "run-no-video", "issue_number", "issue_url", "private"} {
		if bytes.Contains(watchBody, []byte(hidden)) {
			t.Errorf("public snapshot exposed hidden/unplayable data %q: %s", hidden, watchBody)
		}
	}
	if got := boringStatusCalls.Load(); got != 0 {
		t.Fatalf("ordinary finished run triggered %d replay readiness checks, want 0", got)
	}

	var decoded spectatorDashboard
	if err := json.Unmarshal(watchBody, &decoded); err != nil {
		t.Fatalf("decode public snapshot: %v", err)
	}
	if len(decoded.Runs) != 3 {
		t.Fatalf("public runs = %+v, want live + 2 curated replays", decoded.Runs)
	}
	if decoded.Summary.Completed != 4 {
		t.Fatalf("farm summary completed = %d, want all 4 completed runs", decoded.Summary.Completed)
	}

	res, err = http.Get(ui.URL + "/v1/watch/runs/run-goal/replay/status")
	if err != nil {
		t.Fatalf("GET public replay status: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", res.StatusCode, body)
	}
	for _, want := range []string{`"run_id":"run-goal"`, `"state":"ready"`, `"size":6`} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("public status missing %s: %s", want, body)
		}
	}
	for _, secret := range []string{"object_key", "private/runs", "private storage detail", `"error"`} {
		if bytes.Contains(body, []byte(secret)) {
			t.Errorf("public replay status leaked %q: %s", secret, body)
		}
	}

	beforeBlocked := replayCalls.Load()
	for _, runID := range []string{"run-boring", "run-no-video"} {
		res, err := http.Get(ui.URL + "/v1/watch/runs/" + runID + "/replay/status")
		if err != nil {
			t.Fatalf("GET hidden replay status %s: %v", runID, err)
		}
		io.Copy(io.Discard, res.Body) //nolint:errcheck
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("hidden replay status %s = %d, want 404", runID, res.StatusCode)
		}
	}
	if got := replayCalls.Load(); got != beforeBlocked {
		t.Fatalf("hidden replay status reached replay service: %d -> %d", beforeBlocked, got)
	}

	req, err := http.NewRequest(http.MethodGet, ui.URL+"/v1/watch/runs/run-goal/replay/video", nil)
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

	beforeBlocked = replayCalls.Load()
	for _, probe := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/watch/runs/run-goal/replay/render"},
		{http.MethodGet, "/v1/watch/runs/run-goal/artifacts"},
		{http.MethodGet, "/v1/watch/runs/run-goal/debug"},
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

func TestSpectatorWithoutReplayServicePublishesOnlyActiveRuns(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		res.Write([]byte(`{"now":123,"runs":[{"run_id":"live","status":"running"},{"run_id":"done","status":"done","stats":{"goal_complete":true}}]}`)) //nolint:errcheck
	}))
	t.Cleanup(wall.Close)
	ui := httptest.NewServer(spectatorHandler(wall.URL))
	t.Cleanup(ui.Close)

	res, err := http.Get(ui.URL + "/v1/watch")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !bytes.Contains(body, []byte("live")) || bytes.Contains(body, []byte(`"run_id":"done"`)) {
		t.Fatalf("snapshot without replay service = %s, want only active run", body)
	}
}

func TestSpectatorSummaryCountsWholeFarmBeforeCuration(t *testing.T) {
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
