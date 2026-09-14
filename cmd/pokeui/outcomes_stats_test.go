package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOutcomesStatsHandlerUsesNarrowWallFeed(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/outcomes" {
			t.Fatalf("path=%s, want /v1/outcomes", req.URL.Path)
		}
		res.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(res).Encode(map[string]any{"runs": []statsRun{
			{RunID: "done-1", Status: "done", Attempts: 1, Reason: "goal", Player: &statsPlayer{Badges: []string{"boulder"}}},
			{RunID: "done-2", Status: "done", Attempts: 2, Reason: "error"},
		}})
	}))
	defer wall.Close()

	res := httptest.NewRecorder()
	outcomesStatsHandler(wall.URL).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/stats", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var got farmOutcomeStats
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SettledRuns != 2 || got.CompletedAttempts != 3 || got.AtLeastOneBadge != 1 || got.BestBadges != 1 {
		t.Fatalf("stats=%#v", got)
	}
}
