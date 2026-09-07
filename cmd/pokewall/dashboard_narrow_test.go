package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The wall must narrow the run list itself: a client asking for two runs of
// one status must not be handed the whole farm to filter.
func TestDashboardNarrowsByStatusAndLimit(t *testing.T) {
	w := &Wall{tiles: map[string]*Tile{}}
	for _, s := range []struct{ id, status string }{
		{"a", statusDone}, {"b", statusQueued}, {"c", statusDone}, {"d", statusDone},
	} {
		w.tiles[s.id] = &Tile{RunID: s.id, Status: s.status}
		w.order = append(w.order, s.id)
	}

	get := func(query string) []tileRow {
		t.Helper()
		res := httptest.NewRecorder()
		w.handleDashboard(res, httptest.NewRequest(http.MethodGet, "/v1/dashboard"+query, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("GET %q = %d, want 200", query, res.Code)
		}
		var view dashboardView
		if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view.Runs
	}

	if got := get(""); len(got) != 4 {
		t.Fatalf("unnarrowed = %d runs, want all 4", len(got))
	}
	if got := get("?status=" + statusDone); len(got) != 3 {
		t.Fatalf("status filter = %d runs, want 3", len(got))
	}
	if got := get("?limit=2"); len(got) != 2 {
		t.Fatalf("limit=2 = %d runs, want 2", len(got))
	}

	// Status must be applied BEFORE the limit, or a caller asking for two
	// done runs gets fewer than two whenever a queued run sorts ahead.
	got := get("?status=" + statusDone + "&limit=2")
	if len(got) != 2 {
		t.Fatalf("status+limit = %d runs, want 2", len(got))
	}
	for _, r := range got {
		if r.Status != statusDone {
			t.Fatalf("run %q has status %q, want %q", r.RunID, r.Status, statusDone)
		}
	}

	res := httptest.NewRecorder()
	w.handleDashboard(res, httptest.NewRequest(http.MethodGet, "/v1/dashboard?limit=0", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("limit=0 = %d, want 400", res.Code)
	}
}
