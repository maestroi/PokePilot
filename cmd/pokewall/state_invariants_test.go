package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestLoadStateBackfillsReplayAvailableFromDump(t *testing.T) {
	dir := t.TempDir()
	dumps := filepath.Join(dir, "dumps")
	if err := os.Mkdir(dumps, 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(dir, "state.json")
	// Pre-flag wall state: a finished run with no replay_available field.
	if err := os.WriteFile(stateFile, []byte(`{"order":["old-rec"],"queue":[],"tiles":{"old-rec":{"run_id":"old-rec","status":"done","finished":true,"attempts":1,"reason":"failed"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := json.Marshal(farm.FinishReport{
		RunID: "old-rec", Attempt: 1, Reason: "failed",
		Artifacts: []farm.Artifact{recordingArtifact("old-rec")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dumps, "old-rec.json"), report, 0o644); err != nil {
		t.Fatal(err)
	}

	w := NewWall(dumps)
	w.SetStatePath(stateFile)
	got := getDashboard(t, w.Handler())
	if len(got.Runs) != 1 || got.Runs[0].RunID != "old-rec" || !got.Runs[0].ReplayAvailable {
		t.Fatalf("restored dashboard = %+v, want old-rec with replay_available", got.Runs)
	}
}

func TestRestartPreservesReplayAvailableWithoutDumps(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "wall-state.json")
	w1 := NewWall("")
	w1.SetStatePath(stateFile)
	srv1 := httptest.NewServer(w1.Handler())
	enqueueLease(t, w1.Handler(), "kept-rec")
	if resp := postJSON(t, srv1.URL+"/v1/runs/kept-rec/finish", farm.FinishReport{
		RunID: "kept-rec", Attempt: 1, Reason: "failed",
		Artifacts: []farm.Artifact{recordingArtifact("kept-rec")},
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("finish: status %d", resp.StatusCode)
	}
	srv1.Close()

	w2 := NewWall("")
	w2.SetStatePath(stateFile)
	got := getDashboard(t, w2.Handler())
	if len(got.Runs) != 1 || !got.Runs[0].ReplayAvailable {
		t.Fatalf("restored dashboard = %+v, want kept-rec with replay_available from state", got.Runs)
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
