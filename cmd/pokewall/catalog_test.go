package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestCatalogMigratesRAMAndEvictsFinished(t *testing.T) {
	w := NewWall(t.TempDir())
	w.mu.Lock()
	w.order = []string{"done-1", "live-1"}
	w.tiles["done-1"] = &Tile{
		RunID: "done-1", Status: statusDone, Finished: true, Planner: "llm",
		Starter: "squirtle", Attempts: 1, Reason: "goal", EndedAt: time.Unix(20, 0),
	}
	w.tiles["live-1"] = &Tile{
		RunID: "live-1", Status: statusRunning, Planner: "llm", Starter: "bulbasaur",
		lastUpdate: time.Now(),
	}
	w.mu.Unlock()

	path := filepath.Join(t.TempDir(), "catalog.db")
	if err := w.SetCatalogPath(path); err != nil {
		t.Fatal(err)
	}
	defer w.CloseCatalog()

	w.mu.Lock()
	_, finishedInRAM := w.tiles["done-1"]
	_, liveInRAM := w.tiles["live-1"]
	w.mu.Unlock()
	if finishedInRAM {
		t.Fatal("finished tile remained in RAM after catalog migration")
	}
	if !liveInRAM {
		t.Fatal("live tile was evicted")
	}

	row, ok := w.snapshotRun("done-1")
	if !ok || row.RunID != "done-1" || row.Status != statusDone || row.Reason != "goal" {
		t.Fatalf("catalog snapshot = %#v, %v", row, ok)
	}
}

func TestCatalogDashboardDefaultsActiveAndPagesDoneHistory(t *testing.T) {
	w := NewWall(t.TempDir())
	w.mu.Lock()
	w.order = []string{"done-1", "done-2", "live-1"}
	w.tiles["done-1"] = &Tile{RunID: "done-1", Status: statusDone, Finished: true, EndedAt: time.Unix(10, 0), Reason: "lost"}
	w.tiles["done-2"] = &Tile{RunID: "done-2", Status: statusDone, Finished: true, EndedAt: time.Unix(20, 0), Reason: "goal"}
	w.tiles["live-1"] = &Tile{RunID: "live-1", Status: statusRunning, lastUpdate: time.Now()}
	w.mu.Unlock()
	if err := w.SetCatalogPath(filepath.Join(t.TempDir(), "catalog.db")); err != nil {
		t.Fatal(err)
	}
	defer w.CloseCatalog()

	handler := w.catalogHTTPHandler(runtimeOperatorHTTPHandler(w))

	activeRes := httptest.NewRecorder()
	handler.ServeHTTP(activeRes, httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil))
	if activeRes.Code != http.StatusOK {
		t.Fatalf("active status=%d body=%s", activeRes.Code, activeRes.Body.String())
	}
	var active runtimeDashboardView
	if err := json.Unmarshal(activeRes.Body.Bytes(), &active); err != nil {
		t.Fatal(err)
	}
	if len(active.Runs) != 1 || active.Runs[0].RunID != "live-1" {
		t.Fatalf("default dashboard runs=%#v", active.Runs)
	}

	historyRes := httptest.NewRecorder()
	handler.ServeHTTP(historyRes, httptest.NewRequest(http.MethodGet, "/v1/dashboard?status=done&limit=1&offset=0&facets=1", nil))
	if historyRes.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", historyRes.Code, historyRes.Body.String())
	}
	var history runtimeDashboardView
	if err := json.Unmarshal(historyRes.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Total != 2 || len(history.Runs) != 1 || history.Runs[0].RunID != "done-2" {
		t.Fatalf("history=%#v", history)
	}
	if history.Facets == nil || len(history.Facets.Outcomes) != 2 {
		t.Fatalf("facets=%#v", history.Facets)
	}
}

func TestCatalogSurvivesWallRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.db")
	first := NewWall(t.TempDir())
	first.mu.Lock()
	first.order = []string{"done-1"}
	first.tiles["done-1"] = &Tile{RunID: "done-1", Status: statusDone, Finished: true, Attempts: 2, Reason: "goal", EndedAt: time.Unix(30, 0)}
	first.mu.Unlock()
	if err := first.SetCatalogPath(path); err != nil {
		t.Fatal(err)
	}
	if err := first.CloseCatalog(); err != nil {
		t.Fatal(err)
	}

	second := NewWall(t.TempDir())
	if err := second.SetCatalogPath(path); err != nil {
		t.Fatal(err)
	}
	defer second.CloseCatalog()
	row, ok := second.snapshotRun("done-1")
	if !ok || row.Attempts != 2 || row.Reason != "goal" {
		t.Fatalf("restarted catalog row=%#v ok=%v", row, ok)
	}
}
