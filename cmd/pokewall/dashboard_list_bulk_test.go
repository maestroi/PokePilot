package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestLiveDashboardOmitsStrategicRecordsWithoutMutatingTheTile(t *testing.T) {
	w := NewWall("")
	records := []farm.StrategicCallRecord{{DurationSeconds: 1.5, PlanSteps: []string{"progress fly_ready"}}}
	w.mu.Lock()
	w.order = []string{"run-1"}
	w.tiles["run-1"] = &Tile{
		RunID:  "run-1",
		Status: "running",
		Stats: &farm.LLMStats{
			Rounds:                  4,
			StrategicRecordsDropped: 1,
			StrategicRecords:        records,
		},
	}
	w.mu.Unlock()

	view := w.runtimeDashboardSnapshot(runtimeDashboardQuery{active: true, limit: 10})
	if len(view.Runs) != 1 || view.Runs[0].Stats == nil {
		t.Fatalf("live dashboard runs = %+v", view.Runs)
	}
	if got := view.Runs[0].Stats; len(got.StrategicRecords) != 0 || got.Rounds != 4 || got.StrategicRecordsDropped != 1 {
		t.Fatalf("list stats = %+v, want records stripped and the summary kept", got)
	}
	if len(w.tiles["run-1"].Stats.StrategicRecords) != 1 {
		t.Fatal("dashboard list cleared strategic records on the live tile")
	}
	row, ok := w.snapshotRun("run-1")
	if !ok || len(row.Stats.StrategicRecords) != 1 {
		t.Fatalf("single-run snapshot = %+v ok=%v, want the records kept", row.Stats, ok)
	}

	res := httptest.NewRecorder()
	w.handleDashboard(res, httptest.NewRequest(http.MethodGet, "/v1/dashboard?status=running&limit=5", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("GET /v1/dashboard = %d %s", res.Code, res.Body.String())
	}
	var dash dashboardView
	if err := json.Unmarshal(res.Body.Bytes(), &dash); err != nil {
		t.Fatal(err)
	}
	if len(dash.Runs) != 1 || dash.Runs[0].Stats == nil || len(dash.Runs[0].Stats.StrategicRecords) != 0 {
		t.Fatalf("RAM dashboard leaked strategic records: %s", res.Body.String())
	}
	if len(w.tiles["run-1"].Stats.StrategicRecords) != 1 {
		t.Fatal("RAM dashboard cleared strategic records on the live tile")
	}
}
