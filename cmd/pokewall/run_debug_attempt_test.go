package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

// A failing attempt's repro evidence must stay reachable after an endless run
// moved on: /debug?attempt=N describes attempt N, not the run's latest attempt.
func TestRunInspectorDebugTargetsRequestedAttempt(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	w.mu.Lock()
	w.order = append(w.order, "debug-run")
	w.tiles["debug-run"] = &Tile{
		RunID: "debug-run", Status: statusDone, Attempts: 2, Reason: "goal", Detail: "done", Finished: true,
	}
	w.mu.Unlock()

	// Attempt 1 is the failure whose repro bundle must remain reachable;
	// attempt 2 is the run's latest attempt and progressed.
	first := farm.FinishReport{
		RunID: "debug-run", Attempt: 1, Reason: "error",
		ProgressEarly: &farm.Progress{Round: 0, Badges: 0, Events: 1, Maps: 1, Map: 1, MapName: "Pallet Town"},
		ProgressFinal: &farm.Progress{Round: 7, Badges: 0, Events: 2, Maps: 2, Map: 1, MapName: "Pallet Town"},
		Artifacts: []farm.Artifact{
			{Name: "round-007-frame-0000012345-go-to-route-9.failure-repro.json", MediaType: "application/json", SHA256: "first-repro", Data: []byte(`{"attempt":1}`)},
			{Name: "run.gbrun", MediaType: "application/octet-stream", SHA256: "first-recording", Store: farm.ArtifactStoreS3, Bucket: "pokepilot", ObjectKey: "runs/debug-run/attempt-1/run.gbrun", Size: 111},
		},
	}
	second := farm.FinishReport{
		RunID: "debug-run", Attempt: 2, Reason: "goal",
		ProgressEarly: &farm.Progress{Round: 0, Badges: 0, Events: 1, Maps: 1, Map: 1, MapName: "Pallet Town"},
		ProgressFinal: &farm.Progress{Round: 12, Badges: 1, Events: 9, Maps: 8, Map: 3, MapName: "Route 3"},
		Artifacts: []farm.Artifact{
			{Name: "summary.json", MediaType: "application/json", SHA256: "second-summary", Data: []byte(`{"attempt":2}`)},
			{Name: "run.gbrun", MediaType: "application/octet-stream", SHA256: "second-recording", Store: farm.ArtifactStoreS3, Bucket: "pokepilot", ObjectKey: "runs/debug-run/attempt-2/run.gbrun", Size: 222},
		},
	}
	for path, report := range map[string]farm.FinishReport{
		filepath.Join(dir, safeDumpName("debug-run")):  first,
		filepath.Join(dir, "debug-run-attempt-2.json"): second,
	} {
		data, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	srv := httptest.NewServer(wallHTTPHandler(w))
	defer srv.Close()

	get := func(query string) (int, runDebugView) {
		t.Helper()
		res, err := http.Get(srv.URL + "/v1/runs/debug-run/debug" + query)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		var view runDebugView
		if err := json.Unmarshal(body, &view); err != nil {
			t.Fatalf("decode %q: %v (%s)", query, err, body)
		}
		return res.StatusCode, view
	}

	// Latest attempt stays the default when no attempt is requested.
	status, latest := get("")
	if status != http.StatusOK || latest.Finish == nil || latest.Finish.Attempt != 2 {
		t.Fatalf("latest debug status=%d finish=%+v", status, latest.Finish)
	}
	if len(latest.Artifacts) != 2 || latest.Artifacts[1].ObjectKey != "runs/debug-run/attempt-2/run.gbrun" {
		t.Fatalf("latest artifacts = %+v", latest.Artifacts)
	}

	// The failing attempt's finish, artifacts, summary and timeline all
	// describe the requested attempt.
	status, historical := get("?attempt=1")
	if status != http.StatusOK || historical.Finish == nil || historical.Finish.Attempt != 1 || historical.Finish.Reason != "error" {
		t.Fatalf("historical debug status=%d finish=%+v", status, historical.Finish)
	}
	if len(historical.Artifacts) != 2 || historical.Artifacts[0].Name != "round-007-frame-0000012345-go-to-route-9.failure-repro.json" ||
		historical.Artifacts[1].ObjectKey != "runs/debug-run/attempt-1/run.gbrun" {
		t.Fatalf("historical artifacts = %+v", historical.Artifacts)
	}
	if !historical.Summary.ProgressKnown || historical.Summary.BadgeDelta != 0 || !historical.Summary.ReplayAvailable {
		t.Fatalf("historical summary = %+v", historical.Summary)
	}
	if !latest.Summary.ProgressKnown || latest.Summary.BadgeDelta != 1 {
		t.Fatalf("latest summary = %+v", latest.Summary)
	}

	// An invalid selector is a client error, not a silent fallback to latest.
	for _, query := range []string{"?attempt=0", "?attempt=-3", "?attempt=abc"} {
		res, err := http.Get(srv.URL + "/v1/runs/debug-run/debug" + query)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, res.Body) //nolint:errcheck
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", query, res.StatusCode)
		}
	}
}
