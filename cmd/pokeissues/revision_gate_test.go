package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type revisionGateGitHub struct {
	mu             sync.Mutex
	issue          githubIssue
	closedAt       time.Time
	fixedRevision  string
	compareStatus  string
	patched        int
	comments       []githubComment
	commitLookups  int
	compareLookups int
}

func (f *revisionGateGitHub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/o/r/issues", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]githubIssue{f.issue})
	})
	mux.HandleFunc("GET /repos/o/r/issues/{number}", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number":       f.issue.Number,
			"state":        f.issue.State,
			"state_reason": f.issue.StateReason,
			"body":         f.issue.Body,
			"closed_at":    f.closedAt.UTC().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("PATCH /repos/o/r/issues/{number}", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.patched++
		f.issue.State = "open"
		f.issue.StateReason = "reopened"
		issue := f.issue
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(issue)
	})
	mux.HandleFunc("GET /repos/o/r/issues/{number}/comments", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		comments := append([]githubComment(nil), f.comments...)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(comments)
	})
	mux.HandleFunc("POST /repos/o/r/issues/{number}/comments", func(w http.ResponseWriter, r *http.Request) {
		var comment githubComment
		if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.comments = append(f.comments, comment)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(comment)
	})
	mux.HandleFunc("GET /repos/o/r/commits", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.commitLookups++
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode([]githubCommitSummary{{SHA: f.fixedRevision}})
	})
	mux.HandleFunc("GET /repos/o/r/compare/{range}", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.compareLookups++
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(githubCompareResult{Status: f.compareStatus})
	})
	return mux
}

func newRevisionGateClient(t *testing.T, fake *revisionGateGitHub) *githubClient {
	t.Helper()
	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)
	client, err := newGitHubClient(server.URL, "https://github.test", "o/r", "token", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func closedGateIssue(t *testing.T, externalID string) githubIssue {
	t.Helper()
	manifest := sampleManifest(externalID)
	return githubIssue{
		Number:      17,
		State:       "closed",
		StateReason: "completed",
		Body:        renderIssueBody("", manifest, nil),
	}
}

func TestReportKeepsIssueClosedForPreFixRunner(t *testing.T) {
	fake := &revisionGateGitHub{
		issue:         closedGateIssue(t, "original"),
		closedAt:      time.Date(2026, 9, 14, 5, 32, 52, 0, time.UTC),
		fixedRevision: "fix-sha",
		compareStatus: "ahead", // base=old-run, head=fix-sha
	}
	client := newRevisionGateClient(t, fake)
	manifest := sampleManifest("stale-run")
	manifest.ObservedRevision = "old-run-sha"

	got, created, err := client.report(context.Background(), manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created || got.Issue.Status != "resolved" || got.Automation.Warning == "" {
		t.Fatalf("report = %+v created=%v", got, created)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.patched != 0 {
		t.Fatalf("stale recurrence reopened issue: patched=%d", fake.patched)
	}
	if len(fake.comments) != 1 || !strings.Contains(fake.comments[0].Body, "Stale farm recurrence") {
		t.Fatalf("comments = %+v", fake.comments)
	}
	if fake.commitLookups != 1 || fake.compareLookups != 1 {
		t.Fatalf("commit lookups=%d compare lookups=%d", fake.commitLookups, fake.compareLookups)
	}
}

func TestReportReopensWhenFixedBuildItselfReproduces(t *testing.T) {
	fake := &revisionGateGitHub{
		issue:         closedGateIssue(t, "original"),
		closedAt:      time.Date(2026, 9, 14, 5, 32, 52, 0, time.UTC),
		fixedRevision: "fix-sha",
	}
	client := newRevisionGateClient(t, fake)
	manifest := sampleManifest("same-build-run")
	manifest.ObservedRevision = "fix-sha"

	got, _, err := client.report(context.Background(), manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Issue.Status != "open" {
		t.Fatalf("status=%q, want open", got.Issue.Status)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.patched != 1 {
		t.Fatalf("fixed build recurrence patched=%d, want 1", fake.patched)
	}
	if fake.compareLookups != 0 {
		t.Fatalf("equal revisions should not need compare API, got %d calls", fake.compareLookups)
	}
	if len(fake.comments) != 1 || !strings.Contains(fake.comments[0].Body, "Farm regression recurrence") {
		t.Fatalf("comments = %+v", fake.comments)
	}
}

func TestGetIssueStatusPublishesClosureBaseline(t *testing.T) {
	fake := &revisionGateGitHub{
		issue:         closedGateIssue(t, "original"),
		closedAt:      time.Date(2026, 9, 14, 5, 32, 52, 0, time.UTC),
		fixedRevision: "fix-sha",
	}
	client := newRevisionGateClient(t, fake)
	got, err := client.getIssueStatus(context.Background(), "17")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "resolved" || got.Resolution != "fixed" || got.FixedRevision != "fix-sha" {
		t.Fatalf("status = %+v", got)
	}
}
