package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProductionStepAdvancesWithoutRepeatingAssessment(t *testing.T) {
	goalComplete := true
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"run": map[string]any{
			"status": "done", "reason": "done", "stats": map[string]any{"goal_complete": goalComplete},
		}})
	}))
	defer wall.Close()
	s := newReplayServer(wall.URL, "", "", nil)
	var health map[string]any
	if _, err := selfJSON(context.Background(), s.handler(), http.MethodGet, "/healthz", nil, &health); err != nil || health["status"] == nil {
		t.Fatalf("in-process call through the real mux: %v %v", health, err)
	}

	h := func(c byte) string { return strings.Repeat(string(c), 64) }
	broadcast, edits := "generating", "generating"
	calls := map[string]int{}
	var formats []string
	mux := http.NewServeMux()
	route := func(pattern string, reply func(r *http.Request) any) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			calls[pattern]++
			writeJSON(w, http.StatusOK, reply(r))
		})
	}
	route("POST /v1/runs/{id}/replay/render", func(*http.Request) any { return map[string]string{"state": broadcast} })
	route("POST /v1/runs/{id}/producer/assess", func(*http.Request) any {
		return map[string]any{"assessment": map[string]string{"hash": h('a'), "publication_class": "full_episode"}}
	})
	route("POST /v1/runs/{id}/studio/edits", func(*http.Request) any { return map[string]any{"plan": map[string]string{"hash": h('b')}} })
	route("POST /v1/runs/{id}/studio/edits/{hash}/render", func(*http.Request) any { return map[string]string{"state": edits} })
	route("POST /v1/runs/{id}/studio/packages", func(*http.Request) any {
		return map[string]any{"package": map[string]any{"hash": h('c'), "titles": []any{map[string]string{"id": "t"}}, "shorts": []any{map[string]string{"id": "s"}}}}
	})
	route("POST /v1/runs/{id}/studio/packages/{hash}/thumbnails", func(*http.Request) any {
		return map[string]any{"manifest": map[string]any{"candidates": []any{map[string]string{"id": h('d')}}}}
	})
	route("POST /v1/runs/{id}/studio/packages/{hash}/thumbnails/{candidate}/select", func(*http.Request) any { return map[string]string{} })
	route("POST /v1/runs/{id}/studio/publications", func(r *http.Request) any {
		var body struct {
			Format  string `json:"format"`
			Privacy string `json:"privacy"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Privacy != "private" {
			t.Errorf("candidate %s privacy = %q, want private", body.Format, body.Privacy)
		}
		formats = append(formats, body.Format)
		return map[string]string{"id": h('e'), "state": "queued"}
	})

	st := productionState{Version: 1, RunID: "run-e2e", Stage: "pending"}
	step := func() {
		t.Helper()
		if err := s.productionStep(context.Background(), mux, &st); err != nil {
			t.Fatalf("step at %s: %v", st.Stage, err)
		}
	}

	step() // broadcast still rendering
	if st.Stage != "broadcast_render" || st.Done || calls["POST /v1/runs/{id}/producer/assess"] != 0 {
		t.Fatalf("after broadcast wait: %+v calls=%v", st, calls)
	}
	broadcast = "ready"
	step() // episode edit still rendering
	if st.Stage != "episode_render" || st.Done {
		t.Fatalf("after edit wait: %+v", st)
	}
	edits = "ready"
	step()
	if !st.Done || st.Stage != "ready_for_review" || len(st.Candidates) != 2 {
		t.Fatalf("final: %+v", st)
	}
	if n := calls["POST /v1/runs/{id}/producer/assess"]; n != 1 {
		t.Fatalf("producer assessed %d times, want exactly 1 (it is LLM-backed and non-deterministic)", n)
	}
	if strings.Join(formats, ",") != "episode,short" {
		t.Fatalf("candidates = %v", formats)
	}

	goalComplete = false
	st = productionState{Version: 1, RunID: "run-lost", Stage: "pending"}
	step()
	if !st.Done || st.Stage != "goal_not_complete" {
		t.Fatalf("run without the goal flag must stop, got %+v", st)
	}
}

func TestProductionRetryRearmsFailedStageKeepingAssessment(t *testing.T) {
	_, store := newMemS3(t)
	s := newReplayServer("http://wall.invalid", "", "", store)
	ctx := context.Background()
	h := s.handler()
	assessment := strings.Repeat("a", 64)

	if code, _ := selfJSON(ctx, h, http.MethodPost, "/v1/runs/run-e2e/production/retry", nil, nil); code != http.StatusConflict {
		t.Fatalf("retry of pending production = %d, want 409", code)
	}
	if err := s.saveProduction(ctx, productionState{Version: 1, RunID: "run-e2e", Stage: "failed_episode_render", Done: true,
		AssessmentHash: assessment, Verdict: "full_episode", Error: "422", Failures: productionMaxFailures}); err != nil {
		t.Fatal(err)
	}
	var st productionState
	if _, err := selfJSON(ctx, h, http.MethodPost, "/v1/runs/run-e2e/production/retry", nil, &st); err != nil {
		t.Fatal(err)
	}
	if st.Stage != "episode_render" || st.Done || st.Failures != 0 || st.Error != "" || st.AssessmentHash != assessment {
		t.Fatalf("retry state = %+v", st)
	}
	if saved, err := s.loadProduction(ctx, "run-e2e"); err != nil || saved.Done || saved.Stage != "episode_render" {
		t.Fatalf("retry not persisted: %+v %v", saved, err)
	}
}
