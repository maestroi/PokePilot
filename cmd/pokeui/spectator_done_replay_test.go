package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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
