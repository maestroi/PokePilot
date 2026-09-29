package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSpectatorPublishesSuccessfulDoneReplayWhenNothingIsLive(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/v1/dashboard" {
			http.NotFound(res, req)
			return
		}
		res.Header().Set("Content-Type", "application/json")
		switch {
		case req.URL.Query().Get("active") == "1":
			_, _ = res.Write([]byte(`{"now":200,"runs":[]}`))
		case req.URL.Query().Get("status") == "done":
			_, _ = res.Write([]byte(`{"now":200,"runs":[{"run_id":"run-success","status":"done","reason":"done","goal":"Earn the Boulder Badge.","ended_at":199}]}`))
		default:
			http.Error(res, "unexpected dashboard query", http.StatusBadRequest)
		}
	}))
	t.Cleanup(wall.Close)

	replay := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == "/v1/runs/run-success/replay/status" {
			res.Header().Set("Content-Type", "application/json")
			_, _ = res.Write([]byte(`{"run_id":"run-success","state":"ready","size":1234}`))
			return
		}
		http.NotFound(res, req)
	}))
	t.Cleanup(replay.Close)

	ui := httptest.NewServer(spectatorHandlerWithReplay(wall.URL, replay.URL))
	t.Cleanup(ui.Close)

	res, err := http.Get(ui.URL + "/v1/watch")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/watch = %d: %s", res.StatusCode, body)
	}
	for _, want := range [][]byte{
		[]byte(`"run_id":"run-success"`),
		[]byte(`"replay_ready":true`),
		[]byte(`"highlight":"goal complete"`),
		[]byte(`"goal_complete":true`),
	} {
		if !bytes.Contains(body, want) {
			t.Fatalf("public snapshot missing %s: %s", want, body)
		}
	}
}

// A slow sidecar answering "missing" for many candidates must not starve the
// ready replay of the request budget (statuses used to be fetched serially).
func TestSpectatorReadyReplaySurvivesManySlowMissingCandidates(t *testing.T) {
	const candidates = 40
	// publicSpectatorRuns walks the list from the end, so the ready run goes
	// first to be reached last.
	runs := []string{`{"run_id":"run-ready","status":"done","reason":"done","ended_at":1}`}
	for i := 0; i < candidates; i++ {
		runs = append(runs, fmt.Sprintf(`{"run_id":"run-slow-%d","status":"done","reason":"done","ended_at":%d}`, i, 100+i))
	}
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		if req.URL.Query().Get("active") == "1" {
			_, _ = res.Write([]byte(`{"now":200,"runs":[]}`))
			return
		}
		_, _ = res.Write([]byte(`{"now":200,"runs":[` + strings.Join(runs, ",") + `]}`))
	}))
	t.Cleanup(wall.Close)

	replay := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		if strings.Contains(req.URL.Path, "run-ready") {
			_, _ = res.Write([]byte(`{"state":"ready","size":1}`))
			return
		}
		time.Sleep(250 * time.Millisecond) // 41 serial calls ≈ 10s > proxyTimeout
		_, _ = res.Write([]byte(`{"state":"missing"}`))
	}))
	t.Cleanup(replay.Close)

	ui := httptest.NewServer(spectatorHandlerWithReplay(wall.URL, replay.URL))
	t.Cleanup(ui.Close)
	res, err := http.Get(ui.URL + "/v1/watch")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !bytes.Contains(body, []byte(`"run_id":"run-ready"`)) {
		t.Fatalf("ready replay missing from public snapshot: %s", body)
	}
}
