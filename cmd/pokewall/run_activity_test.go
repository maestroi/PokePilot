package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestRunActivityRingIsBoundedAndDeduplicated(t *testing.T) {
	tile := &Tile{RunID: "ring", Status: statusRunning}
	appendRunActivityLocked(tile, runActivityEvent{Source: "skill", Kind: "execution", Summary: "same"})
	appendRunActivityLocked(tile, runActivityEvent{Source: "skill", Kind: "execution", Summary: "same"})
	if len(tile.Activity) != 1 {
		t.Fatalf("duplicate events = %d, want 1", len(tile.Activity))
	}
	for i := 0; i < runActivityKeep+20; i++ {
		appendRunActivityLocked(tile, runActivityEvent{
			Source: "system", Kind: "test", Summary: fmt.Sprintf("event %d", i),
			Detail: fmt.Sprintf("detail %d", i),
		})
	}
	if len(tile.Activity) != runActivityKeep {
		t.Fatalf("activity ring = %d, want %d", len(tile.Activity), runActivityKeep)
	}
}

func TestOperatorInspectorShowsLiveActorsAndRecoveryAcrossRetry(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(wallHTTPHandler(w))
	defer srv.Close()

	runID := "activity-resilient"
	if resp := postJSON(t, srv.URL+"/v1/specs", farm.Spec{
		RunID: runID, Planner: "llm", Goal: farm.GoalFrom("Enter the Hall of Fame."),
		RecoveryProfile: farm.RecoveryProfileResilient,
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("queue: %d", resp.StatusCode)
	}
	if resp := postJSON(t, srv.URL+"/v1/lease", struct{}{}); resp.StatusCode != http.StatusOK {
		t.Fatalf("lease: %d", resp.StatusCode)
	}

	if resp := postJSON(t, srv.URL+"/v1/runs/"+runID+"/heartbeat", farm.Heartbeat{
		RunID: runID, Frame: 100, Map: 1, X: 4, Y: 5,
		Question: "1: travel to Pewter City\n2: train nearby",
		Trace:    "walk: moving north",
		Activity: &farm.ActivityEvent{
			Source: "skill", Kind: "started", Summary: "travel to Pewter City", Frame: 100, Round: 1,
		},
		Stats:  &farm.LLMStats{Round: 1},
		Player: &farm.Player{Party: []farm.PartyMon{{Name: "SQUIRTLE", Level: 8}}},
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("planning heartbeat: %d", resp.StatusCode)
	}
	if resp := postJSON(t, srv.URL+"/v1/runs/"+runID+"/heartbeat", farm.Heartbeat{
		RunID: runID, Frame: 220, Map: 2, X: 8, Y: 3,
		Question: "1: travel to Pewter City\n2: train nearby",
		Decision: "travel to Pewter City",
		Trace:    "travel: entered Pewter City",
		Stats:    &farm.LLMStats{Round: 1},
		Player: &farm.Player{
			Badges: []string{"Boulder"},
			Party:  []farm.PartyMon{{Name: "SQUIRTLE", Level: 12}},
		},
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("decision heartbeat: %d", resp.StatusCode)
	}
	if resp := postJSON(t, srv.URL+"/v1/runs/"+runID+"/finish", farm.FinishReport{
		RunID: runID, Attempt: 1, Reason: "stuck", Detail: "navigation stalled at Route 3",
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("finish/recover: %d", resp.StatusCode)
	}

	res, err := http.Get(srv.URL + "/v1/runs/" + runID + "/debug")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("debug status = %d", res.StatusCode)
	}
	var debug runDebugView
	if err := json.NewDecoder(res.Body).Decode(&debug); err != nil {
		t.Fatal(err)
	}
	if debug.Run.Status != statusQueued || debug.Run.RecoveryProfile != farm.RecoveryProfileResilient || debug.Run.RecoveryAttempts != 1 {
		t.Fatalf("live recovered run = status %q profile %q recovery %d", debug.Run.Status, debug.Run.RecoveryProfile, debug.Run.RecoveryAttempts)
	}

	sources := map[string]bool{}
	kinds := map[string]bool{}
	for _, event := range debug.Timeline {
		sources[event.Source] = true
		kinds[event.Kind] = true
	}
	for _, source := range []string{"system", "llm", "skill", "milestone", "recovery"} {
		if !sources[source] {
			t.Errorf("timeline missing %q source: %+v", source, debug.Timeline)
		}
	}
	for _, kind := range []string{"planning", "decision", "badge", "failure", "retry"} {
		if !kinds[kind] {
			t.Errorf("timeline missing %q event: %+v", kind, debug.Timeline)
		}
	}
}

func TestDurableRunActivityKeepsRecoveryButDropsHeartbeatBreadcrumbs(t *testing.T) {
	events := []runActivityEvent{
		{Source: "llm", Kind: "decision", Summary: "go north"},
		{Source: "skill", Kind: "execution", Summary: "walking"},
		{Source: "recovery", Kind: "rollback", Summary: "rollback"},
		{Source: "milestone", Kind: "badge", Summary: "Boulder Badge"},
		{Source: "system", Kind: "leased", Summary: "leased"},
	}
	got := durableRunActivity(events)
	if len(got) != 3 {
		t.Fatalf("durable activity = %+v", got)
	}
	for _, event := range got {
		if event.Source == "llm" || event.Source == "skill" {
			t.Fatalf("heartbeat breadcrumb leaked into durable control-plane activity: %+v", event)
		}
	}
}
