package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestRunInspectorBuildsCompactDebugBundleAndArtifactBrowser(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	w.mu.Lock()
	w.order = append(w.order, "run-42")
	w.tiles["run-42"] = &Tile{
		RunID: "run-42", Status: statusDone, Planner: "llm", Starter: "squirtle",
		Goal: "Earn the Boulder Badge.", LLMProfile: "gpu", Seed: 42,
		QueuedAt: time.Unix(100, 0), EndedAt: time.Unix(200, 0), Attempts: 1,
		Frame: 12345, Map: 3, X: 7, Y: 8, Question: "1. go north", Decision: "go north",
		Reason: "stagnation", Detail: "no progress", Finished: true,
		Stats: &farm.LLMStats{Round: 12, Calls: 14, Model: "debug-model"},
	}
	w.mu.Unlock()

	report := farm.FinishReport{
		RunID: "run-42", Attempt: 1, Reason: "stagnation", Detail: "no progress",
		RunnerVersion: "abc123", SeedBurn: 3, TraceTail: []string{"trace-a", "trace-b"},
		ProgressEarly: &farm.Progress{Round: 0, Badges: 0, Events: 2, Maps: 3, Map: 1, MapName: "Pallet Town"},
		ProgressFinal: &farm.Progress{Round: 12, Badges: 1, Events: 9, Maps: 8, Map: 3, MapName: "Route 3"},
		Artifacts: []farm.Artifact{
			{Name: "summary.json", MediaType: "application/json", SHA256: "inline", Data: []byte(`{"ok":true}`)},
			{Name: "run.gbrun", MediaType: "application/octet-stream", SHA256: "recording", Store: farm.ArtifactStoreS3, Bucket: "pokepilot", ObjectKey: "runs/run-42/attempt-1/run.gbrun", Size: 1234},
		},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, safeDumpName("run-42")), data, 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(wallHTTPHandler(w))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/v1/runs/run-42/debug")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("debug status = %d body=%s", res.StatusCode, body)
	}
	var debug runDebugView
	if err := json.NewDecoder(res.Body).Decode(&debug); err != nil {
		t.Fatal(err)
	}
	if !debug.Summary.ProgressKnown || !debug.Summary.Progressed || debug.Summary.BadgeDelta != 1 || !debug.Summary.ReplayAvailable {
		t.Fatalf("summary = %+v", debug.Summary)
	}
	if len(debug.Artifacts) != 2 || !debug.Artifacts[1].Replayable || debug.Artifacts[1].ObjectKey == "" {
		t.Fatalf("artifacts = %+v", debug.Artifacts)
	}
	if debug.Finish == nil || len(debug.Finish.TraceTail) != 2 || debug.Finish.RunnerVersion != "abc123" {
		t.Fatalf("finish = %+v", debug.Finish)
	}

	res, err = http.Get(srv.URL + "/v1/runs/run-42/artifacts/summary.json/content")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != `{"ok":true}` {
		t.Fatalf("inline content status=%d body=%q", res.StatusCode, body)
	}

	res, err = http.Get(srv.URL + "/v1/runs/run-42/artifacts/run.gbrun/content")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("remote content status=%d, want 409", res.StatusCode)
	}
}

func TestRunInspectorCanReadDumpAfterHistoryRowWasDeleted(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	report := farm.FinishReport{RunID: "old-run", Attempt: 2, Reason: "goal", Detail: "done"}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "old-run-attempt-2.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(wallHTTPHandler(w))
	defer srv.Close()
	res, err := http.Get(srv.URL + "/v1/runs/old-run")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Run tileRow `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Run.RunID != "old-run" || out.Run.Attempts != 2 || out.Run.Status != statusDone {
		t.Fatalf("run = %+v", out.Run)
	}
}
