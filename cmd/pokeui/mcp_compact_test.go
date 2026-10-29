package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPListRunsSummaryCutsContextWithoutLosingProgress(t *testing.T) {
	bulk := strings.Repeat("strategy and decision context ", 400)
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/dashboard" {
			http.NotFound(res, req)
			return
		}
		if req.URL.Query().Get("limit") != fmt.Sprint(mcpListPageSize) {
			t.Errorf("dashboard limit = %q, want %d", req.URL.Query().Get("limit"), mcpListPageSize)
		}
		_ = json.NewEncoder(res).Encode(map[string]any{
			"now":     123,
			"workers": []map[string]any{{"addr": "private-worker:1234", "version": "secret"}},
			"runs": []map[string]any{{
				"run_id": "run-compact", "status": "running", "game": "pokemon-gold",
				"planner": "llm", "goal": "Earn badge 8", "frame": 321,
				"trace": bulk, "question": bulk, "decision": bulk,
				"game_state":    map[string]any{"verbose": bulk},
				"game_decision": map[string]any{"verbose": bulk},
				"stats": map[string]any{
					"rounds": 7, "goal_current": 3, "goal_target": 8,
					"strategic_records": []map[string]any{{"reason": bulk}},
				},
				"player": map[string]any{"money": 100, "badges": []string{"Zephyr"}, "party": []any{
					map[string]any{"species": "Totodile", "moves": []string{"Scratch"}},
				}},
				"issue": map[string]any{
					"issue_number": 42, "status": "closed", "resolution": "fixed", "private_notes": bulk,
				},
			}},
		})
	}))
	defer wall.Close()

	control := &mcpControl{wallBase: wall.URL, http: wall.Client()}
	_, compact, err := control.listRuns(context.Background(), nil, mcpListRunsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if compact["view"] != "summary" {
		t.Fatalf("default view = %v", compact["view"])
	}
	if _, ok := compact["workers"]; ok {
		t.Fatal("summary leaked worker addresses")
	}
	runs, ok := compact["runs"].([]map[string]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("summary runs = %#v", compact["runs"])
	}
	r := runs[0]
	for _, field := range []string{"trace", "question", "decision", "game_state", "game_decision"} {
		if _, ok := r[field]; ok {
			t.Fatalf("summary leaked %s", field)
		}
	}
	if r["frame"] != uint64(321) || r["game"] != "pokemon-gold" {
		t.Fatalf("progress missing from summary: %#v", r)
	}
	if r["issue"].(map[string]any)["resolution"] != "fixed" {
		t.Fatalf("issue resolution missing: %#v", r["issue"])
	}
	if _, ok := r["issue"].(map[string]any)["private_notes"]; ok {
		t.Fatal("issue notes leaked")
	}
	_, full, err := control.listRuns(context.Background(), nil, mcpListRunsInput{View: "full"})
	if err != nil {
		t.Fatal(err)
	}
	fullRuns := full["runs"].([]mcpRunView)
	if len(fullRuns) != 1 || fullRuns[0].Trace != bulk || fullRuns[0].GameDecision["verbose"] != bulk {
		t.Fatal("explicit full view lost legacy fields")
	}
	if _, ok := full["workers"]; !ok {
		t.Fatal("full view lost worker metadata")
	}
	smallJSON, _ := json.Marshal(compact)
	fullJSON, _ := json.Marshal(full)
	if len(smallJSON)*10 >= len(fullJSON) {
		t.Fatalf("summary should be less than 10%% of full output: %d vs %d bytes", len(smallJSON), len(fullJSON))
	}
}

func TestMCPGetRunSummaryAndFull(t *testing.T) {
	bulk := strings.Repeat("internal planning ", 150)
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/runs/test-run" {
			http.NotFound(res, req)
			return
		}
		_ = json.NewEncoder(res).Encode(map[string]any{
			"run": map[string]any{
				"run_id": "test-run", "status": "running", "frame": 88,
				"detail": bulk, "question": bulk, "trace": bulk,
				"activity": []any{map[string]any{"detail": bulk}},
				"stats": map[string]any{
					"round": 3, "calls": 9, "goal_current": 2,
					"strategic_records": []any{bulk},
				},
				"player": map[string]any{"badges": []string{"Zephyr"}, "money": 90,
					"party": []any{map[string]any{"species": "Totodile", "debug": bulk}}},
			},
			"upstream_internal": bulk,
		})
	}))
	defer wall.Close()

	control := &mcpControl{wallBase: wall.URL, http: wall.Client()}
	_, summary, err := control.getRun(context.Background(), nil, mcpRunDetailInput{RunID: "test-run"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := summary["upstream_internal"]; ok {
		t.Fatal("summary leaked wall-only fields")
	}
	run := summary["run"].(map[string]any)
	for _, key := range []string{"trace", "question", "activity"} {
		if _, ok := run[key]; ok {
			t.Fatalf("summary leaked %s", key)
		}
	}
	if run["frame"] != float64(88) || run["party_size"] != 1 {
		t.Fatalf("summary lost progress: %#v", run)
	}
	if len(run["detail"].(string)) > 243 {
		t.Fatal("detail was not clipped")
	}
	if _, ok := run["stats"].(map[string]any)["strategic_records"]; ok {
		t.Fatal("summary leaked stats history")
	}
	_, full, err := control.getRun(context.Background(), nil, mcpRunDetailInput{RunID: "test-run", View: "full"})
	if err != nil {
		t.Fatal(err)
	}
	if full["run"].(map[string]any)["question"] != bulk {
		t.Fatal("full view did not preserve planner question")
	}
	if _, _, err := control.getRun(context.Background(), nil, mcpRunDetailInput{RunID: "test-run", View: "gigantic"}); err == nil {
		t.Fatal("invalid view was accepted")
	}
}

func TestMCPTriageSummaryPaginationAndFullHistory(t *testing.T) {
	groups := make([]map[string]any, 0, 27)
	for i := 0; i < 27; i++ {
		group := map[string]any{
			"key": fmt.Sprintf("failure-%02d", i), "fingerprint": "stable",
			"example": strings.Repeat("irrelevant stack trace ", 80),
			"run_ids": []string{"a", "b", "c", "d", "e"},
			"issue":   map[string]any{"issue_number": i + 1, "status": "open"},
		}
		if i >= 25 {
			group["issue"] = map[string]any{"issue_number": i + 1, "status": "closed", "resolution": "fixed"}
		}
		groups = append(groups, group)
	}
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/triage" {
			http.NotFound(res, req)
			return
		}
		_ = json.NewEncoder(res).Encode(groups)
	}))
	defer wall.Close()

	control := &mcpControl{wallBase: wall.URL, http: wall.Client()}
	_, first, err := control.getTriage(context.Background(), nil, mcpTriageInput{})
	if err != nil {
		t.Fatal(err)
	}
	page := first["groups"].([]map[string]any)
	if len(page) != 20 || first["total"] != 25 || first["next_offset"] != 20 || first["resolved_hidden"] != 2 {
		t.Fatalf("first page metadata = %#v", first)
	}
	if len(page[0]["run_ids"].([]any)) != 3 || page[0]["run_ids_omitted"] != 2 {
		t.Fatalf("run ids were not bounded: %#v", page[0])
	}
	if len(page[0]["example"].(string)) > 243 {
		t.Fatal("failure example was not clipped")
	}
	_, second, err := control.getTriage(context.Background(), nil, mcpTriageInput{Offset: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(second["groups"].([]map[string]any)) != 5 {
		t.Fatalf("second page = %#v", second)
	}
	if _, ok := second["next_offset"]; ok {
		t.Fatal("last page should not have next_offset")
	}
	_, full, err := control.getTriage(context.Background(), nil, mcpTriageInput{IncludeResolved: true, View: "full"})
	if err != nil {
		t.Fatal(err)
	}
	fullGroups := full["groups"].([]map[string]any)
	if len(fullGroups) != 27 || len(fullGroups[0]["run_ids"].([]any)) != 5 {
		t.Fatal("full view did not preserve historical evidence")
	}
	if _, _, err := control.getTriage(context.Background(), nil, mcpTriageInput{Offset: -1}); err == nil {
		t.Fatal("negative offset accepted")
	}
	if _, _, err := control.getTriage(context.Background(), nil, mcpTriageInput{View: "all"}); err == nil {
		t.Fatal("unknown view accepted")
	}
}
