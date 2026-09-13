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
}
