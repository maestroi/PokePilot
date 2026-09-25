package main

import "testing"

func TestReconcileIssueSinkDropsForeignRemoteBindings(t *testing.T) {
	w := NewWall("")
	w.issueLinks["old"] = IssueLink{
		IssueID:     "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		IssueNumber: 91,
		IssueURL:    "https://orchestrator.example/issues/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Status:      "open",
	}
	w.issueLinks["github"] = IssueLink{
		IssueID:     "42",
		IssueNumber: 42,
		IssueURL:    "https://github.com/maestroi/PokePilot/issues/42",
		Status:      "open",
	}
	w.issueLinks["local-only"] = IssueLink{LastObservedRun: "run-1"}
	w.outbox["old-complete"] = outboxEntry{ExternalID: "old-complete", Key: "old", Status: outboxComplete}
	w.outbox["old-quarantined"] = outboxEntry{ExternalID: "old-quarantined", Key: "old", Status: outboxQuarantined}
	w.outbox["old-error"] = outboxEntry{ExternalID: "old-error", Key: "old", Status: outboxError}
	w.outbox["old-pending"] = outboxEntry{ExternalID: "old-pending", Key: "old", Status: outboxPending}
	w.outbox["github-complete"] = outboxEntry{ExternalID: "github-complete", Key: "github", Status: outboxComplete}

	if got := w.reconcileIssueSink("https://github.com/maestroi/PokePilot/"); got != 1 {
		t.Fatalf("removed=%d, want 1", got)
	}
	if _, ok := w.issueLinks["old"]; ok {
		t.Fatal("foreign Orchestrator binding was retained")
	}
	if _, ok := w.issueLinks["github"]; !ok {
		t.Fatal("matching GitHub binding was removed")
	}
	if _, ok := w.issueLinks["local-only"]; !ok {
		t.Fatal("local-only failure metadata was removed")
	}
	for _, id := range []string{"old-complete", "old-quarantined", "old-error"} {
		if _, ok := w.outbox[id]; ok {
			t.Fatalf("stale terminal outbox %q was retained", id)
		}
	}
	if _, ok := w.outbox["old-pending"]; !ok {
		t.Fatal("pending occurrence was removed instead of being retried against the new sink")
	}
	if _, ok := w.outbox["github-complete"]; !ok {
		t.Fatal("matching GitHub occurrence was removed")
	}
}
