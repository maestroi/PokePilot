package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestNormalizeFailureDetailMatchesWallContract(t *testing.T) {
	want := "still on map <hex> at (<n>,<n>) after <n> frames"
	for _, detail := range []string{
		"still on map 0x0c at (10,35) after 180 frames",
		"still on map 0x21 at (4,22) after 240 frames",
	} {
		if got := normalizeFailureDetail(detail); got != want {
			t.Fatalf("normalizeFailureDetail(%q) = %q, want %q", detail, got, want)
		}
	}
	if got := len(normalizeFailureDetail(strings.Repeat("x", failurePatternCap+20))); got != failurePatternCap {
		t.Fatalf("normalized length = %d, want %d", got, failurePatternCap)
	}
}

func TestMatchingRunsUsesExactTriagePattern(t *testing.T) {
	pattern := normalizeFailureDetail("still on map 0x0c at (10,35)")
	runs := []cleanupRun{
		{RunID: "new", Status: "done", Reason: "error", Detail: "still on map 0x21 at (4,22)", EndedAt: 20},
		{RunID: "old", Status: "done", Reason: "lost", Detail: "still on map 0x0c at (10,35)", EndedAt: 10},
		{RunID: "other", Status: "done", Reason: "error", Detail: "blacked out", EndedAt: 5},
		{RunID: "success", Status: "done", Reason: "done", Detail: "still on map 0x0c at (10,35)", EndedAt: 30},
		{RunID: "active", Status: "running", Reason: "error", Detail: "still on map 0x0c at (10,35)"},
	}
	got := matchingRuns(runs, pattern)
	if len(got) != 2 || got[0].RunID != "old" || got[1].RunID != "new" {
		t.Fatalf("matching runs = %+v, want old,new", got)
	}
}

func groupWithIssue(key string, issue int64, pattern string) triageGroup {
	group := triageGroup{Key: key, Pattern: pattern, Count: 2}
	group.Issue = &struct {
		IssueNumber int64  `json:"issue_number"`
		Status      string `json:"status"`
		Resolution  string `json:"resolution"`
	}{IssueNumber: issue, Status: "fixed", Resolution: "fixed"}
	return group
}

func TestSelectTriageGroupAcceptsIssueNumber(t *testing.T) {
	groups := []triageGroup{
		groupWithIssue("aaaaaaaaaaaaaaaa", 231, "first"),
		groupWithIssue("bbbbbbbbbbbbbbbb", 232, "second"),
	}
	got, err := selectTriageGroup(groups, "", 232)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "bbbbbbbbbbbbbbbb" {
		t.Fatalf("key = %q, want bbbbbbbbbbbbbbbb", got.Key)
	}
}

func TestSelectTriageGroupExplainsIssueNumberPassedAsKey(t *testing.T) {
	_, err := selectTriageGroup(nil, "232", 0)
	if err == nil || !strings.Contains(err.Error(), "use -issue 232") {
		t.Fatalf("error = %v, want -issue hint", err)
	}
}

func TestSelectTriageGroupRejectsAmbiguousSelector(t *testing.T) {
	_, err := selectTriageGroup(nil, "abc", 232)
	if err == nil || !strings.Contains(err.Error(), "either -key or -issue") {
		t.Fatalf("error = %v, want selector conflict", err)
	}
}

func TestRunDryRunAndApplyDeleteThroughOperatorAPI(t *testing.T) {
	pattern := normalizeFailureDetail("still on map 0x0c at (10,35)")
	group := groupWithIssue("abc123", 232, pattern)
	var mu sync.Mutex
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/triage":
			_ = json.NewEncoder(w).Encode([]triageGroup{group})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/dashboard" && r.URL.Query().Get("status") == "done":
			_ = json.NewEncoder(w).Encode(cleanupDashboard{Runs: []cleanupRun{
				{RunID: "run-a", Status: "done", Reason: "error", Detail: "still on map 0x0c at (10,35)"},
				{RunID: "run-b", Status: "done", Reason: "lost", Detail: "still on map 0x21 at (4,22)"},
				{RunID: "run-c", Status: "done", Reason: "error", Detail: "another bug"},
			}})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/runs/"):
			mu.Lock()
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/v1/runs/"))
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"deleted"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var dry bytes.Buffer
	if err := run(context.Background(), server.Client(), server.URL, "", 232, false, &dry); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(dry.String(), "triage abc123 · issue #232") || !strings.Contains(dry.String(), "matching finished runs: 2") || !strings.Contains(dry.String(), "dry run only") {
		t.Fatalf("dry output = %q", dry.String())
	}
	if len(deleted) != 0 {
		t.Fatalf("dry run deleted %v", deleted)
	}

	var applied bytes.Buffer
	if err := run(context.Background(), server.Client(), server.URL, "", 232, true, &applied); err != nil {
		t.Fatalf("apply run: %v", err)
	}
	mu.Lock()
	sort.Strings(deleted)
	gotDeleted := append([]string(nil), deleted...)
	mu.Unlock()
	if strings.Join(gotDeleted, ",") != "run-a,run-b" {
		t.Fatalf("deleted = %v, want run-a,run-b", gotDeleted)
	}
	if !strings.Contains(applied.String(), "Deleting 2 / 2") || !strings.Contains(applied.String(), "deleted: 2") {
		t.Fatalf("apply output = %q", applied.String())
	}
}
