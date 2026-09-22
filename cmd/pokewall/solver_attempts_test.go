package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSolverAttemptRouteUpsertsAndKeepsAttribution(t *testing.T) {
	w := NewWall("")
	w.mu.Lock()
	w.issueLinks["deadbeef"] = IssueLink{IssueID: "42", IssueNumber: 42, Status: "open"}
	w.mu.Unlock()

	post := func(body map[string]any) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/triage/deadbeef/solver-attempt", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		w.Handler().ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	post(map[string]any{
		"id": "attempt-1", "backend": "opencode", "model": "qwen3.8-27b/qwen3.8-27b",
		"state": "started", "run_id": "run-1",
	})
	post(map[string]any{
		"id": "attempt-1", "backend": "opencode", "model": "qwen3.8-27b/qwen3.8-27b",
		"state": "pr_opened", "branch": "fix/example", "pr_number": 77,
		"pr_url": "https://github.com/maestroi/PokePilot/pull/77",
	})

	w.mu.Lock()
	link := w.issueLinks["deadbeef"]
	w.mu.Unlock()
	if len(link.SolverAttempts) != 1 {
		t.Fatalf("solver attempts=%d, want 1: %+v", len(link.SolverAttempts), link.SolverAttempts)
	}
	got := link.SolverAttempts[0]
	if got.Model != "qwen3.8-27b/qwen3.8-27b" || got.State != "pr_opened" || got.PRNumber != 77 {
		t.Fatalf("attempt=%+v", got)
	}
	if got.RunID != "run-1" || got.StartedAt == 0 || got.UpdatedAt == 0 {
		t.Fatalf("attempt lost start metadata: %+v", got)
	}
}

func TestSolverAttemptRouteRejectsUnknownFailure(t *testing.T) {
	w := NewWall("")
	req := httptest.NewRequest(http.MethodPost, "/v1/triage/missing/solver-attempt", bytes.NewBufferString(`{"id":"a","backend":"opencode","state":"started"}`))
	res := httptest.NewRecorder()
	w.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}
