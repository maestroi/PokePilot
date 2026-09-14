package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOutcomesCompatibilityServesRAMWhenCatalogDisabled(t *testing.T) {
	w := NewWall(t.TempDir())
	w.mu.Lock()
	w.order = []string{"done-1", "live-1"}
	w.tiles["done-1"] = &Tile{
		RunID: "done-1", Status: statusDone, Finished: true, Planner: "llm",
		Starter: "squirtle", Attempts: 2, Reason: "goal", EndedAt: time.Unix(20, 0),
	}
	w.tiles["live-1"] = &Tile{
		RunID: "live-1", Status: statusRunning, Planner: "llm", Starter: "bulbasaur",
		lastUpdate: time.Now(),
	}
	w.mu.Unlock()

	next := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		http.NotFound(res, req)
	})
	handler := w.outcomesCompatibility(w.catalogHTTPHandler(next))

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/outcomes", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var got struct {
		Runs []catalogOutcomeRun `json:"runs"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Runs) != 2 {
		t.Fatalf("runs=%#v", got.Runs)
	}
	if got.Runs[0].RunID != "done-1" || got.Runs[0].Status != statusDone || got.Runs[0].Attempts != 2 || got.Runs[0].Reason != "goal" {
		t.Fatalf("done run=%#v", got.Runs[0])
	}
	if got.Runs[1].RunID != "live-1" || got.Runs[1].Status != statusRunning {
		t.Fatalf("live run=%#v", got.Runs[1])
	}
}
