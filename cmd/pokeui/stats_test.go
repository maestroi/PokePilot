package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSummarizeOutcomesCountsProgressAndRetryFailures(t *testing.T) {
	runs := []statsRun{
		{
			RunID: "win", Status: "done", Planner: "llm", Goal: "Earn 8 badges", LLMProfile: "auto",
			Endless: true, RandomSeed: true, MaxRounds: 500, Attempts: 1, Reason: "done",
			Player: &statsPlayer{Badges: []string{"Boulder"}},
			Stats:  &statsLLM{GoalSummary: "1/8 badges", GoalCurrent: 8, GoalTarget: 8, GoalComplete: true},
		},
		{
			RunID: "budget", Status: "done", Planner: "llm", Goal: "Earn 8 badges", LLMProfile: "auto",
			Endless: true, RandomSeed: true, MaxRounds: 500, Attempts: 3, Reason: "budget",
			Player: &statsPlayer{Badges: []string{"Boulder", "Cascade"}},
			Stats:  &statsLLM{GoalSummary: "2/8 badges", GoalCurrent: 2, GoalTarget: 8},
		},
		{
			RunID: "error", Status: "done", Planner: "llm", Goal: "Earn 8 badges", LLMProfile: "auto",
			Endless: true, RandomSeed: true, MaxRounds: 500, Attempts: 3, Reason: "error",
		},
		{
			RunID: "retrying", Status: "running", Planner: "llm", Goal: "Earn 8 badges", LLMProfile: "auto",
			Endless: true, RandomSeed: true, MaxRounds: 500, Attempts: 1,
		},
	}

	got := summarizeOutcomes(runs)
	if got.CompletedAttempts != 8 {
		t.Fatalf("completed attempts = %d, want 8", got.CompletedAttempts)
	}
	if got.SettledRuns != 3 {
		t.Fatalf("settled runs = %d, want 3", got.SettledRuns)
	}
	if got.RetryableFailureAttempts != 6 {
		t.Fatalf("retryable failures = %d, want 6", got.RetryableFailureAttempts)
	}
	if got.UsableProgressRuns != 2 || got.AtLeastOneBadge != 2 || got.BestBadges != 2 {
		t.Fatalf("progress summary = usable %d, >=1 %d, best %d", got.UsableProgressRuns, got.AtLeastOneBadge, got.BestBadges)
	}
	if got.GoalTrackedRuns != 2 || got.GoalWins != 1 {
		t.Fatalf("goal summary = tracked %d wins %d", got.GoalTrackedRuns, got.GoalWins)
	}
	if got.BadgeDistribution[1].Count != 1 || got.BadgeDistribution[2].Count != 1 {
		t.Fatalf("badge distribution = %+v", got.BadgeDistribution)
	}
	if len(got.EndlessExperiments) != 1 {
		t.Fatalf("endless experiments = %d, want 1", len(got.EndlessExperiments))
	}
	exp := got.EndlessExperiments[0]
	if exp.CompletedAttempts != 8 || exp.RetryableFailureAttempts != 6 || exp.BestBadges != 2 {
		t.Fatalf("endless experiment = %+v", exp)
	}

	reasons := map[string]int{}
	for _, r := range got.TerminalReasons {
		reasons[r.Name] = r.Count
	}
	for _, name := range []string{"done", "budget", "error"} {
		if reasons[name] != 1 {
			t.Fatalf("reason %q = %d, want 1", name, reasons[name])
		}
	}
}

func TestSummarizeOutcomesTreatsOldSettledTileAsOneAttempt(t *testing.T) {
	got := summarizeOutcomes([]statsRun{{Status: "done", Reason: "budget", Attempts: 0}})
	if got.CompletedAttempts != 1 {
		t.Fatalf("completed attempts = %d, want 1", got.CompletedAttempts)
	}
	if got.RetryableFailureAttempts != 0 {
		t.Fatalf("retryable failures = %d, want 0", got.RetryableFailureAttempts)
	}
}

func TestStatsHandlerReadsDashboardAndDisablesCaching(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/dashboard" {
			t.Fatalf("path = %q", req.URL.Path)
		}
		_ = json.NewEncoder(res).Encode(statsDashboard{Runs: []statsRun{{
			Status: "done", Attempts: 1, Reason: "budget", Player: &statsPlayer{Badges: []string{"Boulder"}},
		}}})
	}))
	defer wall.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	res := httptest.NewRecorder()
	statsHandler(wall.URL).ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var got farmOutcomeStats
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if got.CompletedAttempts != 1 || got.AtLeastOneBadge != 1 || got.BestBadges != 1 {
		t.Fatalf("stats = %+v", got)
	}
}
