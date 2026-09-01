package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestRestartQueuedRunLeasesExactlyOnce(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "wall-state.json")

	w1 := NewWall("")
	w1.SetStatePath(stateFile)
	srv1 := httptest.NewServer(w1.Handler())
	if resp := postJSON(t, srv1.URL+"/v1/specs", spec("restart-once")); resp.StatusCode != http.StatusOK {
		t.Fatalf("enqueue: status %d", resp.StatusCode)
	}
	srv1.Close()

	w2 := NewWall("")
	w2.SetStatePath(stateFile)
	srv2 := httptest.NewServer(w2.Handler())
	defer srv2.Close()

	resp := postJSON(t, srv2.URL+"/v1/lease", struct{}{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lease after restart: status %d", resp.StatusCode)
	}
	var got farm.Spec
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode lease: %v", err)
	}
	if got.RunID != "restart-once" || got.Attempt != 1 {
		t.Fatalf("lease = %+v, want restart-once attempt 1", got)
	}
	if resp := postJSON(t, srv2.URL+"/v1/lease", struct{}{}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("second lease: status %d, want 204", resp.StatusCode)
	}
}

func TestRestartDoesNotRequeueLeasedRun(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "wall-state.json")

	w1 := NewWall("")
	w1.SetStatePath(stateFile)
	srv1 := httptest.NewServer(w1.Handler())
	if resp := postJSON(t, srv1.URL+"/v1/specs", spec("in-flight")); resp.StatusCode != http.StatusOK {
		t.Fatalf("enqueue: status %d", resp.StatusCode)
	}
	if resp := postJSON(t, srv1.URL+"/v1/lease", struct{}{}); resp.StatusCode != http.StatusOK {
		t.Fatalf("lease: status %d", resp.StatusCode)
	}
	srv1.Close()

	w2 := NewWall("")
	w2.SetStatePath(stateFile)
	srv2 := httptest.NewServer(w2.Handler())
	defer srv2.Close()
	if resp := postJSON(t, srv2.URL+"/v1/lease", struct{}{}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("lease after restart: status %d, want 204 for already leased run", resp.StatusCode)
	}
}

func TestRestartPreservesFinishedRunWithoutRequeue(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "wall-state.json")

	w1 := NewWall("")
	w1.SetStatePath(stateFile)
	srv1 := httptest.NewServer(w1.Handler())
	postJSON(t, srv1.URL+"/v1/specs", spec("finished"))
	postJSON(t, srv1.URL+"/v1/lease", struct{}{})
	if resp := postJSON(t, srv1.URL+"/v1/runs/finished/finish", farm.FinishReport{RunID: "finished", Attempt: 1, Reason: "done"}); resp.StatusCode != http.StatusOK {
		t.Fatalf("finish: status %d", resp.StatusCode)
	}
	srv1.Close()

	w2 := NewWall("")
	w2.SetStatePath(stateFile)
	if tile := w2.tiles["finished"]; tile == nil || !tile.Finished || tile.Status != statusDone || tile.Attempts != 1 {
		t.Fatalf("restored tile = %+v, want finished done attempt 1", tile)
	}
	srv2 := httptest.NewServer(w2.Handler())
	defer srv2.Close()
	if resp := postJSON(t, srv2.URL+"/v1/lease", struct{}{}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("lease after finished restart: status %d, want 204", resp.StatusCode)
	}
}

func FuzzWallSpecJSONDoesNotPanic(f *testing.F) {
	f.Add([]byte(`{"run_id":"fuzz-seed","planner":"scripted"}`))
	f.Add([]byte(`{"run_id":`))
	f.Add([]byte(`null`))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, body []byte) {
		w := NewWall("")
		req := httptest.NewRequest(http.MethodPost, "/v1/specs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		w.Handler().ServeHTTP(res, req)
		if res.Code < 200 || res.Code >= 500 {
			t.Fatalf("unexpected status %d for body %q", res.Code, body)
		}
	})
}
