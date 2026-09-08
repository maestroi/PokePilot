package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestStructuredReportersCountOneQuarantinedOccurrence(t *testing.T) {
	var calls atomic.Int32
	ao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "active fingerprint must remain quarantined", http.StatusInternalServerError)
	}))
	t.Cleanup(ao.Close)

	dir := t.TempDir()
	w := NewWall(dir)
	w.issues = newIssueClient(ao.URL, "p", "http://ui", time.Second)
	failure, occurrence := structuredObjectiveFailureFixture(t, "build-a")
	w.mu.Lock()
	w.issueLinks[occurrence.Key] = IssueLink{
		IssueID:     "issue-active",
		IssueNumber: 9,
		IssueURL:    "http://ui/issues/issue-active",
		Status:      "open",
		Fingerprint: occurrence.Fingerprint,
	}
	w.mu.Unlock()

	artifact, err := farm.NewObjectiveFailureArtifact([]farm.ObjectiveFailure{failure})
	if err != nil {
		t.Fatalf("NewObjectiveFailureArtifact: %v", err)
	}
	dump := farm.FinishReport{
		RunID:         "run-active",
		Attempt:       1,
		Reason:        "error",
		Detail:        farm.FailureDetailMarker(occurrence),
		RunnerVersion: "build-a",
		Artifacts:     []farm.Artifact{artifact},
	}
	raw, err := json.Marshal(dump)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, safeDumpName(dump.RunID)), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	general := outboxEntry{
		ExternalID: "run-active-attempt-1",
		RunID:      dump.RunID,
		Attempt:    1,
		Key:        occurrence.Key,
		Status:     outboxPending,
	}
	w.mu.Lock()
	w.outbox[general.ExternalID] = general
	w.mu.Unlock()

	if err := w.dispatchOccurrence(general); err != nil {
		t.Fatalf("generic dispatch: %v", err)
	}
	if err := w.reportObjectiveFailure(dump, failure); err != nil {
		t.Fatalf("objective dispatch: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("orchestrator calls = %d, want 0", calls.Load())
	}

	w.mu.Lock()
	link := w.issueLinks[occurrence.Key]
	generic := w.outbox[general.ExternalID]
	objective := w.outbox[objectiveFailureExternalID(dump.RunID, dump.Attempt, occurrence.Key)]
	w.mu.Unlock()
	if link.QuarantinedCount != 1 {
		t.Fatalf("quarantined count = %d, want one occurrence across both reporters", link.QuarantinedCount)
	}
	if generic.Status != outboxQuarantined || objective.Status != outboxQuarantined {
		t.Fatalf("outbox states generic=%+v objective=%+v", generic, objective)
	}
}
