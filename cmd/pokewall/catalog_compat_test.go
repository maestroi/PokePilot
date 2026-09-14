package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestCatalogTriageSurvivesFinishedTileEviction(t *testing.T) {
	w := NewWall(t.TempDir())
	detail := "route blocked at map 0x12 x=4 y=9"
	pattern := normalizeDetail(detail)
	key, _ := failureIdentity(pattern)

	w.mu.Lock()
	w.order = []string{"failed-1", "failed-2"}
	w.tiles["failed-1"] = &Tile{RunID: "failed-1", Status: statusDone, Finished: true, Reason: "error", Detail: detail, EndedAt: time.Unix(10, 0)}
	w.tiles["failed-2"] = &Tile{RunID: "failed-2", Status: statusDone, Finished: true, Reason: "error", Detail: "route blocked at map 0x99 x=8 y=2", EndedAt: time.Unix(20, 0)}
	w.issueLinks[key] = IssueLink{IssueID: "313", Status: "open"}
	w.mu.Unlock()

	if err := w.SetCatalogPath(filepath.Join(t.TempDir(), "catalog.db")); err != nil {
		t.Fatal(err)
	}
	defer w.CloseCatalog()
	w.mu.Lock()
	if len(w.tiles) != 0 {
		t.Fatalf("finished tiles remained in RAM: %d", len(w.tiles))
	}
	w.mu.Unlock()

	handler := w.catalogOperatorCompatibility(w.catalogHTTPHandler(runtimeOperatorHTTPHandler(w)))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/triage", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var groups []triageGroup
	if err := json.Unmarshal(res.Body.Bytes(), &groups); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Count != 2 || groups[0].Key != key {
		t.Fatalf("groups=%#v", groups)
	}
	if len(groups[0].RunIDs) != 2 || groups[0].RunIDs[0] != "failed-2" {
		t.Fatalf("newest run sample=%#v", groups[0].RunIDs)
	}
	if groups[0].Issue == nil || groups[0].Issue.IssueID != "313" {
		t.Fatalf("issue overlay=%#v", groups[0].Issue)
	}
}

func TestCatalogHistoryUsesCurrentIssueLink(t *testing.T) {
	w := NewWall(t.TempDir())
	detail := "menu unreachable after move learning"
	key, _ := failureIdentity(normalizeDetail(detail))
	w.mu.Lock()
	w.order = []string{"failed-1"}
	w.tiles["failed-1"] = &Tile{RunID: "failed-1", Status: statusDone, Finished: true, Reason: "error", Detail: detail, EndedAt: time.Unix(10, 0)}
	w.mu.Unlock()
	if err := w.SetCatalogPath(filepath.Join(t.TempDir(), "catalog.db")); err != nil {
		t.Fatal(err)
	}
	defer w.CloseCatalog()

	// The issue is created/updated after the run has already been cataloged.
	w.mu.Lock()
	w.issueLinks[key] = IssueLink{IssueID: "github-313", Status: "closed", Resolution: "fixed"}
	w.mu.Unlock()

	handler := w.catalogOperatorCompatibility(w.catalogHTTPHandler(runtimeOperatorHTTPHandler(w)))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/dashboard?status=done&limit=10", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var dashboard runtimeDashboardView
	if err := json.Unmarshal(res.Body.Bytes(), &dashboard); err != nil {
		t.Fatal(err)
	}
	if len(dashboard.Runs) != 1 || dashboard.Runs[0].Issue == nil {
		t.Fatalf("dashboard runs=%#v", dashboard.Runs)
	}
	if dashboard.Runs[0].Issue.IssueID != "github-313" || dashboard.Runs[0].Issue.Resolution != "fixed" {
		t.Fatalf("issue=%#v", dashboard.Runs[0].Issue)
	}

	inspect := httptest.NewRecorder()
	handler.ServeHTTP(inspect, httptest.NewRequest(http.MethodGet, "/v1/runs/failed-1", nil))
	if inspect.Code != http.StatusOK {
		t.Fatalf("inspect status=%d body=%s", inspect.Code, inspect.Body.String())
	}
	var envelope struct {
		Run tileRow `json:"run"`
	}
	if err := json.Unmarshal(inspect.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Run.Issue == nil || envelope.Run.Issue.Status != "closed" {
		t.Fatalf("inspect issue=%#v", envelope.Run.Issue)
	}
}
