package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestTerminalRunFailureSynthesizesStuck(t *testing.T) {
	dump := farm.FinishReport{
		RunID:         "run-stuck",
		Attempt:       1,
		Reason:        "stuck",
		RunnerVersion: "deadbeef",
		ProgressFinal: &farm.Progress{Round: 12, Map: 0x0c},
	}
	failure, ok := terminalRunFailure(dump, nil)
	if !ok {
		t.Fatal("terminalRunFailure(stuck) = no failure, want synthetic progression blocker")
	}
	if !failure.Blocking || failure.TerminalCount != 1 || failure.Count != 1 {
		t.Fatalf("impact = blocking=%v terminal=%d count=%d, want true/1/1", failure.Blocking, failure.TerminalCount, failure.Count)
	}
	if failure.Map != 0x0c || failure.FirstRound != 12 || failure.LastRound != 12 {
		t.Fatalf("location = map=%02x rounds=%d..%d, want 0c 12..12", failure.Map, failure.FirstRound, failure.LastRound)
	}
	if failure.Outcome != "stuck" || failure.Cause != "stagnation" {
		t.Fatalf("classification = outcome=%q cause=%q", failure.Outcome, failure.Cause)
	}
	if !strings.Contains(failure.Error, "stagnation watchdog") {
		t.Fatalf("error = %q, want stagnation watchdog diagnosis", failure.Error)
	}
	if failure.Build != "deadbeef" {
		t.Fatalf("build = %q, want deadbeef", failure.Build)
	}
}

func TestTerminalRunFailureDoesNotDuplicateSpecificTerminalFailure(t *testing.T) {
	dump := farm.FinishReport{RunID: "run-stuck", Attempt: 1, Reason: "stuck"}
	existing := []farm.ObjectiveFailure{{
		Objective:     "go to saffron city",
		Error:         "route remained blocked",
		Count:         1,
		TerminalCount: 1,
		Blocking:      true,
	}}
	if _, ok := terminalRunFailure(dump, existing); ok {
		t.Fatal("terminalRunFailure duplicated an existing terminal objective failure")
	}
}

func TestTerminalRunFailureIgnoresNormalStops(t *testing.T) {
	for _, reason := range []string{"done", "budget", "lost", ""} {
		if _, ok := terminalRunFailure(farm.FinishReport{Reason: reason}, nil); ok {
			t.Fatalf("terminalRunFailure(%q) synthesized an issue", reason)
		}
	}
}

func TestTerminalRunFailureFallsBackForFailedRuns(t *testing.T) {
	failure, ok := terminalRunFailure(farm.FinishReport{
		Reason:        "failed",
		Detail:        "same objective failed repeatedly",
		RunnerVersion: "cafebabe",
		ProgressFinal: &farm.Progress{Round: 7, Map: 0x03},
	}, nil)
	if !ok {
		t.Fatal("terminalRunFailure(failed) = no failure")
	}
	if failure.Outcome != "failed" || failure.Cause != "failure-budget" || failure.Map != 0x03 {
		t.Fatalf("failure = %+v", failure)
	}
	if !strings.Contains(failure.Error, "same objective failed repeatedly") {
		t.Fatalf("error = %q, want original detail", failure.Error)
	}
}

func TestStuckFinishDumpReportsIssue(t *testing.T) {
	var reports atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reports.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issue": map[string]any{
				"id":           "dddddddd-dddd-dddd-dddd-dddddddddddd",
				"issue_number": 88,
				"status":       "open",
			},
			"occurrence": map[string]any{
				"id":          "occ-stuck",
				"external_id": "run-stuck-attempt-1",
			},
			"automation": map[string]any{"status": "captured"},
		})
	}))
	defer sink.Close()

	dir := t.TempDir()
	w := NewWall(dir)
	w.issues = newIssueClient(sink.URL, "proj", "http://ui", time.Second)
	dump := farm.FinishReport{
		RunID:         "run-stuck",
		Attempt:       1,
		Reason:        "stuck",
		RunnerVersion: "deadbeef",
		ProgressFinal: &farm.Progress{Round: 12, Map: 0x0c},
		TraceTail:     []string{"round 12: no material progress"},
	}
	data, err := json.Marshal(dump)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "run-stuck.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	retry, err := w.reportObjectiveFailureDump(path)
	if err != nil {
		t.Fatalf("reportObjectiveFailureDump: %v", err)
	}
	if retry {
		t.Fatal("reportObjectiveFailureDump requested retry after successful report")
	}
	if reports.Load() != 1 {
		t.Fatalf("issue reports = %d, want 1", reports.Load())
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.issueLinks) != 1 {
		t.Fatalf("issue links = %d, want 1", len(w.issueLinks))
	}
	for _, link := range w.issueLinks {
		if link.IssueNumber != 88 || link.Status != "open" || link.LastReportedRun != "run-stuck" {
			t.Fatalf("issue link = %+v", link)
		}
	}
	if len(w.outbox) != 1 {
		t.Fatalf("outbox = %d, want 1 completed occurrence", len(w.outbox))
	}
	for _, entry := range w.outbox {
		if entry.Status != outboxComplete || entry.RunID != "run-stuck" {
			t.Fatalf("outbox entry = %+v", entry)
		}
	}
}
