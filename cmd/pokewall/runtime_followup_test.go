package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeDashboardPagesAndFiltersHistory(t *testing.T) {
	w := NewWall("")
	now := time.Now()
	w.mu.Lock()
	w.order = []string{"done-1", "active", "done-2", "done-3", "done-4"}
	w.tiles["done-1"] = &Tile{RunID: "done-1", Status: statusDone, Planner: "llm", Reason: "error", Starter: "bulbasaur", EndedAt: now.Add(-4 * time.Minute)}
	w.tiles["active"] = &Tile{RunID: "active", Status: statusRunning, Planner: "llm", Starter: "squirtle"}
	w.tiles["done-2"] = &Tile{RunID: "done-2", Status: statusDone, Planner: "scripted", Reason: "done", Starter: "squirtle", EndedAt: now.Add(-3 * time.Minute)}
	w.tiles["done-3"] = &Tile{RunID: "done-3", Status: statusDone, Planner: "llm", Reason: "done", Starter: "charmander", EndedAt: now.Add(-2 * time.Minute)}
	w.tiles["done-4"] = &Tile{RunID: "done-4", Status: statusDone, Planner: "scripted", Reason: "done", Starter: "squirtle", EndedAt: now.Add(-time.Minute)}
	w.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard?status=done&limit=2&offset=1&facets=1", nil)
	rec := httptest.NewRecorder()
	w.handleRuntimeDashboard(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var page runtimeDashboardView
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	if page.Total != 4 {
		t.Fatalf("total=%d want 4", page.Total)
	}
	if len(page.Runs) != 2 || page.Runs[0].RunID != "done-3" || page.Runs[1].RunID != "done-2" {
		t.Fatalf("page runs=%+v", page.Runs)
	}
	if page.Facets == nil || len(page.Facets.Outcomes) != 2 || len(page.Facets.Hows) != 2 || len(page.Facets.Starters) != 3 {
		t.Fatalf("facets=%+v", page.Facets)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/dashboard?status=done&how=walk&starter=squirtle&limit=1", nil)
	rec = httptest.NewRecorder()
	w.handleRuntimeDashboard(rec, req)
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode filtered page: %v", err)
	}
	if page.Total != 2 || len(page.Runs) != 1 || page.Runs[0].RunID != "done-4" {
		t.Fatalf("filtered total=%d runs=%+v", page.Total, page.Runs)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/dashboard?active=1", nil)
	rec = httptest.NewRecorder()
	w.handleRuntimeDashboard(rec, req)
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode active: %v", err)
	}
	if page.Total != 1 || len(page.Runs) != 1 || page.Runs[0].RunID != "active" {
		t.Fatalf("active total=%d runs=%+v", page.Total, page.Runs)
	}
}

func TestRuntimeDeleteRemovesExactAttemptDumps(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	w.mu.Lock()
	w.order = []string{"run-clean", "run-cleaner"}
	w.tiles["run-clean"] = &Tile{RunID: "run-clean", Status: statusDone, Finished: true, Attempts: 3}
	w.tiles["run-cleaner"] = &Tile{RunID: "run-cleaner", Status: statusDone, Finished: true, Attempts: 1}
	w.mu.Unlock()

	paths := []string{
		filepath.Join(dir, safeDumpName("run-clean")),
		filepath.Join(dir, safeBase("run-clean")+"-attempt-2.json"),
		filepath.Join(dir, safeBase("run-clean")+"-attempt-3.json"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte(`{"run_id":"run-clean"}`), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	other := filepath.Join(dir, safeDumpName("run-cleaner"))
	if err := os.WriteFile(other, []byte(`{"run_id":"run-cleaner"}`), 0o644); err != nil {
		t.Fatalf("write other: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/run-clean", nil)
	req.SetPathValue("id", "run-clean")
	rec := httptest.NewRecorder()
	w.handleRuntimeDelete(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("dump still exists %s: %v", path, err)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("neighbor run dump was removed: %v", err)
	}
	if _, ok := w.snapshotRun("run-clean"); ok {
		t.Fatal("deleted run remains in wall state")
	}
	if _, ok := w.snapshotRun("run-cleaner"); !ok {
		t.Fatal("neighbor run was removed")
	}
}

func TestObjectiveFailureDumpPathsScansOnlyJSONOnce(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.json", "a.json", "ignore.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "nested.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	paths, err := objectiveFailureDumpPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || filepath.Base(paths[0]) != "a.json" || filepath.Base(paths[1]) != "b.json" {
		t.Fatalf("paths=%v", paths)
	}
}
