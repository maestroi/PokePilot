package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

// leaseGoal enqueues a raw spec body, leases it, and reports the goal state
// the wall handed out. It goes through the public endpoints only.
func leaseGoal(t *testing.T, body string) (goal string, provided bool) {
	t.Helper()
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/v1/specs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enqueue status = %d", resp.StatusCode)
	}

	leased, err := farm.NewClient(srv.URL).Lease(context.Background())
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	if leased == nil {
		t.Fatal("lease returned no spec")
	}
	return leased.Goal.String(), leased.Goal.Provided()
}

// TestLeasePreservesUnsetGoal pins that a queued spec with no goal stays
// goal-less after a lease instead of acquiring an explicit empty goal.
func TestLeasePreservesUnsetGoal(t *testing.T) {
	goal, provided := leaseGoal(t, `{"run_id":"goal-unset","planner":"llm"}`)
	if provided || goal != "" {
		t.Fatalf("leased goal = %q provided=%v, want unset", goal, provided)
	}
}

// TestLeasePreservesFreePlayGoal is the regression for a Free play run behind a
// play style that would otherwise supply a default: the explicit empty goal
// must survive the lease unchanged.
func TestLeasePreservesFreePlayGoal(t *testing.T) {
	goal, provided := leaseGoal(t, `{"run_id":"goal-free-play","planner":"llm","play_style":"completionist","goal":""}`)
	if !provided || goal != "" {
		t.Fatalf("leased goal = %q provided=%v, want provided empty", goal, provided)
	}
}

// TestLeasePreservesExplicitGoal covers the ordinary case.
func TestLeasePreservesExplicitGoal(t *testing.T) {
	goal, provided := leaseGoal(t, `{"run_id":"goal-explicit","planner":"llm","goal":"badges:1"}`)
	if !provided || goal != "badges:1" {
		t.Fatalf("leased goal = %q provided=%v, want badges:1", goal, provided)
	}
}

// TestLeaseCarriesRunPolicyFromTheTile pins that policy reaches the runner off
// the tile alone, with no registry lookup.
func TestLeaseCarriesRunPolicyFromTheTile(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	body := `{"run_id":"policy-lease","planner":"llm","goal":"badges:1","play_style":"adventure","purpose":"debug_coverage","risk_tolerance":"balanced","wild_encounters":"fight"}`
	resp, err := http.Post(srv.URL+"/v1/specs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	resp.Body.Close()

	leased, err := farm.NewClient(srv.URL).Lease(context.Background())
	if err != nil || leased == nil {
		t.Fatalf("lease: %v (spec %v)", err, leased)
	}
	if leased.PlayStyle != "adventure" || leased.Purpose != farm.RunPurposeDebugCoverage || leased.RiskTolerance != "balanced" || leased.WildEncounters != "fight" {
		t.Fatalf("leased policy = %q/%q/%q/%q", leased.PlayStyle, leased.Purpose, leased.RiskTolerance, leased.WildEncounters)
	}
}

// TestPersistedStateCarriesRunPolicyAndGoalState covers a wall restart: policy
// and the goal distinction must survive the state file.
func TestPersistedStateCarriesRunPolicyAndGoalState(t *testing.T) {
	w := NewWall("")
	w.tiles["round-trip"] = &Tile{}
	w.order = append(w.order, "round-trip")
	w.applySpec("round-trip", farm.Spec{
		RunID:          "round-trip",
		Planner:        "llm",
		Goal:           farm.GoalFrom(""),
		PlayStyle:      "completionist",
		Purpose:        farm.RunPurposeDebugCoverage,
		RiskTolerance:  "cautious",
		WildEncounters: "planner",
	})

	w.mu.Lock()
	blob, err := w.marshalStateLocked()
	w.mu.Unlock()
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}

	statePath := filepath.Join(t.TempDir(), "wall-state.json")
	if err := os.WriteFile(statePath, blob, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	restored := NewWall("")
	restored.SetStatePath(statePath)

	tile := restored.tiles["round-trip"]
	if tile == nil {
		t.Fatal("restored tile missing")
	}
	if tile.PlayStyle != "completionist" || tile.Purpose != farm.RunPurposeDebugCoverage || tile.RiskTolerance != "cautious" || tile.WildEncounters != "planner" {
		t.Fatalf("restored policy = %q/%q/%q/%q", tile.PlayStyle, tile.Purpose, tile.RiskTolerance, tile.WildEncounters)
	}
	if !tile.GoalProvided || tile.Goal != "" {
		t.Fatalf("restored goal = %q provided=%v, want provided empty", tile.Goal, tile.GoalProvided)
	}
}
