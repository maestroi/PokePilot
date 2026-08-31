package main

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
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

func TestIssueReportMultipart(t *testing.T) {
	var got issueReportManifest
	var files []string
	var hashes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects/proj/issue-reports" {
			t.Errorf("path = %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "multipart/form-data" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
			http.Error(w, "bad type", 400)
			return
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("part: %v", err)
				return
			}
			body, _ := io.ReadAll(part)
			switch part.FormName() {
			case "report":
				if err := json.Unmarshal(body, &got); err != nil {
					t.Errorf("report json: %v", err)
				}
			case "artifact":
				files = append(files, part.FileName())
				sum := hashedNamed(part.FileName(), "application/octet-stream", body).SHA256
				hashes = append(hashes, sum)
			}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"issue":        map[string]any{"id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "issue_number": 42, "status": "open"},
			"occurrence":   map[string]any{"id": "occ-1", "external_id": got.ExternalID},
			"deduplicated": false,
			"automation":   map[string]any{"status": "captured"},
		})
	}))
	t.Cleanup(srv.Close)

	c := newIssueClient(srv.URL, "proj", "http://192.168.50.81:8081", time.Second)
	art := hashedNamed("final.state", "application/octet-stream", []byte("state-bytes"))
	res, err := c.Report(t.Context(), issueReportManifest{
		Source:           "pokefarm",
		Fingerprint:      "sha256:" + strings.Repeat("ab", 32),
		ExternalID:       "run-2f-attempt-1",
		Title:            "stuck",
		Summary:          "stuck",
		ObservedRevision: "abc123",
		Evidence:         json.RawMessage(`{"run_id":"run-2f","attempt":1}`),
	}, []farm.Artifact{art})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if got.Source != "pokefarm" || got.ExternalID != "run-2f-attempt-1" || got.ObservedRevision != "abc123" {
		t.Fatalf("manifest = %+v", got)
	}
	if len(files) != 1 || files[0] != "final.state" || hashes[0] != art.SHA256 {
		t.Fatalf("artifacts = %v hashes = %v", files, hashes)
	}
	if res.Issue.IssueNumber != 42 || res.Automation.Status != "captured" {
		t.Fatalf("response = %+v", res)
	}
	if c.issueURL(res.Issue.ID) != "http://192.168.50.81:8081/issues/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("issue url = %s", c.issueURL(res.Issue.ID))
	}
}

func TestIssueReportAcceptsAutomationStatuses(t *testing.T) {
	for _, status := range []string{"captured", "investigation_started", "investigation_failed"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{
				"issue":      map[string]any{"id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "issue_number": 1, "status": "open"},
				"occurrence": map[string]any{"id": "o", "external_id": "x"},
				"automation": map[string]any{"status": status, "warning": "if any"},
			})
		}))
		c := newIssueClient(srv.URL, "p", "http://ui", time.Second)
		res, err := c.Report(t.Context(), issueReportManifest{Source: "pokefarm", Fingerprint: "sha256:" + strings.Repeat("cd", 32), ExternalID: "e1", Title: "t", Evidence: json.RawMessage(`{}`)}, nil)
		srv.Close()
		if err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		if res.Issue.ID == "" || res.Automation.Status != status {
			t.Fatalf("%s: %+v", status, res)
		}
	}
}

func settleFailedRun(t *testing.T, srvURL, runID, detail string, extra farm.FinishReport) {
	t.Helper()
	for i := 1; i <= maxAttempts; i++ {
		if resp := postJSON(t, srvURL+"/v1/lease", struct{}{}); resp.StatusCode != http.StatusOK {
			t.Fatalf("lease attempt %d: %d", i, resp.StatusCode)
		}
		report := extra
		report.RunID = runID
		report.Attempt = i
		report.Reason = "error"
		report.Detail = detail
		if resp := postJSON(t, srvURL+"/v1/runs/"+runID+"/finish", report); resp.StatusCode != http.StatusOK {
			t.Fatalf("finish attempt %d: %d", i, resp.StatusCode)
		}
	}
}
func TestDispatchOccurrenceFromDump(t *testing.T) {
	var reports atomic.Int32
	ao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reports.Add(1)
		json.NewEncoder(w).Encode(map[string]any{
			"issue":      map[string]any{"id": "cccccccc-cccc-cccc-cccc-cccccccccccc", "issue_number": 7, "status": "open"},
			"occurrence": map[string]any{"id": "o", "external_id": "fail-1-attempt-3"},
			"automation": map[string]any{"status": "captured"},
		})
	}))
	t.Cleanup(ao.Close)

	dir := t.TempDir()
	w := NewWall(dir)
	w.issues = newIssueClient(ao.URL, "proj", "http://ui:8081", time.Second)
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	postJSON(t, srv.URL+"/v1/specs", spec("fail-1"))
	art := hashedNamed("round-001-frame-0000000100-goto.state", "application/octet-stream", []byte("ckpt"))
	know := hashedNamed("round-001-frame-0000000100-goto.knowledge-v4.json", "application/json", []byte(`{}`))
	settleFailedRun(t, srv.URL, "fail-1", "still on map 0x0c at (10,35)", farm.FinishReport{
		SaveState: []byte("final"), RunnerVersion: "deadbeef", SeedBurn: 12,
		Artifacts: []farm.Artifact{art, know},
	})

	e, ok := w.nextOutbox()
	if !ok {
		t.Fatal("no outbox entry after failed finish")
	}
	if err := w.dispatchOccurrence(e); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if reports.Load() != 1 {
		t.Fatalf("reports = %d, want 1", reports.Load())
	}
	if err := w.dispatchOccurrence(e); err != nil {
		t.Fatalf("idempotent dispatch: %v", err)
	}
	key, _ := failureIdentity(normalizeDetail("still on map 0x0c at (10,35)"))
	w.mu.Lock()
	link := w.issueLinks[key]
	w.mu.Unlock()
	if link.IssueNumber != 7 || !strings.Contains(link.IssueURL, "cccccccc-cccc-cccc-cccc-cccccccccccc") {
		t.Fatalf("link = %+v", link)
	}

	groups := w.triage()
	if len(groups) != 1 || groups[0].Issue == nil || groups[0].Issue.IssueNumber != 7 {
		t.Fatalf("triage = %+v", groups)
	}
}

func TestIssueStatusSyncPreservesOnFailure(t *testing.T) {
	var fail atomic.Bool
	ao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": "id-1", "issue_number": 42, "status": "resolved",
			"resolution": "fixed", "occurrence_count": 3, "fixed_revision": "abc",
		})
	}))
	t.Cleanup(ao.Close)
	w := NewWall("")
	w.issues = newIssueClient(ao.URL, "p", "http://ui", time.Second)
	key := "abcdabcdabcdabcd"
	w.mu.Lock()
	w.issueLinks[key] = IssueLink{IssueID: "id-1", IssueNumber: 42, Status: "open", IssueURL: "http://ui/issues/id-1"}
	w.mu.Unlock()

	w.syncIssueStatuses()
	w.mu.Lock()
	got := w.issueLinks[key]
	w.mu.Unlock()
	if got.Status != "resolved" || got.Resolution != "fixed" || got.FixedRevision != "abc" || got.OccurrenceCount != 3 {
		t.Fatalf("synced = %+v", got)
	}

	fail.Store(true)
	w.syncIssueStatuses()
	w.mu.Lock()
	got = w.issueLinks[key]
	w.mu.Unlock()
	if got.Status != "resolved" || !got.Stale {
		t.Fatalf("after failure = %+v", got)
	}
}

func TestInvestigateNow(t *testing.T) {
	var called atomic.Bool
	ao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/investigate") {
			called.Store(true)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(ao.Close)
	w := wallWithFinished(failedTile("run-a", "still on map 0x0c at (10,35)"))
	key, _ := failureIdentity(normalizeDetail("still on map 0x0c at (10,35)"))
	w.issues = newIssueClient(ao.URL, "p", "http://ui", time.Second)
	w.mu.Lock()
	w.issueLinks[key] = IssueLink{IssueID: "id-1", IssueNumber: 42, IssueURL: "http://ui/issues/id-1", Status: "open"}
	w.mu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/v1/triage/"+key+"/investigate", nil)
	res := httptest.NewRecorder()
	w.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("investigate = %d: %s", res.Code, res.Body.String())
	}
	if !called.Load() {
		t.Fatal("orchestrator investigate was not called")
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/triage/missing/investigate", nil)
	res = httptest.NewRecorder()
	w.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("missing key = %d, want 404", res.Code)
	}
}

func TestParseIssueFlags(t *testing.T) {
	c, err := parseIssueFlags("", "", "")
	if err != nil || c != nil {
		t.Fatalf("empty = %v %v, want nil nil", c, err)
	}
	if _, err := parseIssueFlags("http://api", "", ""); err == nil {
		t.Fatal("partial config must be an error")
	}
	c, err = parseIssueFlags("http://api", "proj", "http://ui")
	if err != nil || c == nil {
		t.Fatalf("full = %v %v", c, err)
	}
}

func TestNoOutboxWhenIssuesDisabled(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	postJSON(t, srv.URL+"/v1/specs", spec("no-ao"))
	settleFailedRun(t, srv.URL, "no-ao", "still on map 0x0c at (10,35)", farm.FinishReport{SaveState: []byte("s")})
	if _, ok := w.nextOutbox(); ok {
		t.Fatal("unconfigured wall queued an issue report")
	}
	w.mu.Lock()
	n := len(w.outbox)
	w.mu.Unlock()
	if n != 0 {
		t.Fatalf("outbox size = %d, want 0 when issue integration is disabled", n)
	}
}

func TestPermanentOutboxErrorsAreNotRetried(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	w.issues = newIssueClient("http://127.0.0.1:1", "p", "http://ui", time.Second)
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	postJSON(t, srv.URL+"/v1/specs", spec("bad-hash"))
	settleFailedRun(t, srv.URL, "bad-hash", "blacked out", farm.FinishReport{
		SaveState: []byte("final"),
		Artifacts: []farm.Artifact{hashedNamed("round-001-frame-0000000100-goto.state", "application/octet-stream", []byte("ckpt"))},
	})
	dumpPath := filepath.Join(dir, "bad-hash-attempt-3.json")
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	var dump farm.FinishReport
	if err := json.Unmarshal(data, &dump); err != nil {
		t.Fatalf("decode dump: %v", err)
	}
	if len(dump.Artifacts) == 0 {
		t.Fatal("dump has no artifacts to corrupt")
	}
	dump.Artifacts[0].SHA256 = strings.Repeat("ab", 32)
	corrupted, err := json.Marshal(dump)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(dumpPath, corrupted, 0o644); err != nil {
		t.Fatalf("rewrite dump: %v", err)
	}

	e, ok := w.nextOutbox()
	if !ok {
		t.Fatal("expected pending outbox after failed finish")
	}
	err = w.dispatchOccurrence(e)
	if err == nil {
		t.Fatal("dispatch of corrupt evidence must fail")
	}
	w.noteOutboxResult(e.ExternalID, err, isRetryableIssueError(err))
	if _, ok := w.nextOutbox(); ok {
		t.Fatal("permanent outbox error must not be retried automatically")
	}
	groups := w.triage()
	if len(groups) != 1 || groups[0].Outbox != outboxError {
		t.Fatalf("triage outbox = %+v, want error", groups)
	}
}

func TestDispatchDoesNotHoldMutexDuringHTTP(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	ao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		json.NewEncoder(w).Encode(map[string]any{
			"issue":      map[string]any{"id": "dddddddd-dddd-dddd-dddd-dddddddddddd", "issue_number": 1, "status": "open"},
			"occurrence": map[string]any{"id": "o", "external_id": "x"},
			"automation": map[string]any{"status": "captured"},
		})
	}))
	t.Cleanup(ao.Close)
	dir := t.TempDir()
	w := NewWall(dir)
	w.issues = newIssueClient(ao.URL, "p", "http://ui", 5*time.Second)
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)
	postJSON(t, srv.URL+"/v1/specs", spec("lock-1"))
	settleFailedRun(t, srv.URL, "lock-1", "blacked out", farm.FinishReport{SaveState: []byte("s")})
	e, ok := w.nextOutbox()
	if !ok {
		t.Fatal("no outbox")
	}
	done := make(chan error, 1)
	go func() { done <- w.dispatchOccurrence(e) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP never started")
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil)
	res := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() {
		w.Handler().ServeHTTP(res, req)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("dashboard blocked while issue HTTP in flight; mutex held?")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("dispatch: %v", err)
	}
}
