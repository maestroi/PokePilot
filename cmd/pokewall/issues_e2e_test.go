package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

const (
	e2eIssueID  = "11111111-1111-1111-1111-111111111111"
	e2eIssueNum = int64(42)
	e2eProject  = "proj-e2e"
	e2eUI       = "http://ui.example/issues-ui"
	e2eFixedSHA = "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
)

type capturedIssueReport struct {
	Status   int
	Manifest issueReportManifest
	Files    map[string][]byte
	Hashes   map[string]string
}

type fakeOrchestrator struct {
	mu         sync.Mutex
	reports    []capturedIssueReport
	byExternal map[string]capturedIssueReport
	status     string
	resolution string
	fixedRev   string
	occ        int64
	failNext   int
}

func newFakeOrchestrator() *fakeOrchestrator {
	return &fakeOrchestrator{
		byExternal: map[string]capturedIssueReport{},
		status:     "open",
	}
}

func (f *fakeOrchestrator) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/projects/{project}/issue-reports", f.handleReport)
	mux.HandleFunc("GET /api/issues/{id}", f.handleGet)
	mux.HandleFunc("POST /api/issues/{id}/investigate", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func (f *fakeOrchestrator) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("project") != e2eProject {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext > 0 {
		f.failNext--
		http.Error(w, "temporary", http.StatusBadGateway)
		return
	}
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "multipart/form-data" {
		http.Error(w, "bad type", http.StatusBadRequest)
		return
	}
	reader := multipart.NewReader(r.Body, params["boundary"])
	var rec capturedIssueReport
	rec.Files = map[string][]byte{}
	rec.Hashes = map[string]string{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(part)
		switch part.FormName() {
		case "report":
			if err := json.Unmarshal(body, &rec.Manifest); err != nil {
				http.Error(w, "bad report", http.StatusBadRequest)
				return
			}
		case "artifact":
			name := part.FileName()
			rec.Files[name] = body
			sum := sha256.Sum256(body)
			rec.Hashes[name] = hex.EncodeToString(sum[:])
		}
	}
	if prev, ok := f.byExternal[rec.Manifest.ExternalID]; ok {
		rec.Status = http.StatusOK
		f.reports = append(f.reports, rec)
		json.NewEncoder(w).Encode(map[string]any{
			"issue":        map[string]any{"id": e2eIssueID, "issue_number": e2eIssueNum, "status": f.status},
			"occurrence":   map[string]any{"id": "occ-" + rec.Manifest.ExternalID, "external_id": rec.Manifest.ExternalID},
			"deduplicated": true,
			"automation":   map[string]any{"status": "captured"},
		})
		_ = prev
		return
	}
	f.occ++
	if f.status == "resolved" || f.resolution == "fixed" {
		f.status = "open"
		f.resolution = ""
		f.fixedRev = ""
	}
	rec.Status = http.StatusCreated
	f.byExternal[rec.Manifest.ExternalID] = rec
	f.reports = append(f.reports, rec)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"issue":        map[string]any{"id": e2eIssueID, "issue_number": e2eIssueNum, "status": f.status},
		"occurrence":   map[string]any{"id": "occ-" + rec.Manifest.ExternalID, "external_id": rec.Manifest.ExternalID},
		"deduplicated": false,
		"automation":   map[string]any{"status": "captured"},
	})
}

func (f *fakeOrchestrator) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") != e2eIssueID {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	json.NewEncoder(w).Encode(map[string]any{
		"id":               e2eIssueID,
		"issue_number":     e2eIssueNum,
		"status":           f.status,
		"resolution":       f.resolution,
		"occurrence_count": f.occ,
		"fixed_revision":   f.fixedRev,
	})
}

func (f *fakeOrchestrator) snapshot() (reports []capturedIssueReport, occ int64, status, resolution, fixed string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]capturedIssueReport, len(f.reports))
	copy(out, f.reports)
	return out, f.occ, f.status, f.resolution, f.fixedRev
}

func TestIssueHandoffEndToEnd(t *testing.T) {
	ao := newFakeOrchestrator()
	aoSrv := httptest.NewServer(ao.handler())
	t.Cleanup(aoSrv.Close)

	dumps := t.TempDir()
	stateFile := filepath.Join(t.TempDir(), "state.json")
	w := NewWall(dumps)
	w.SetStatePath(stateFile)
	w.SetIssueClient(newIssueClient(aoSrv.URL, e2eProject, e2eUI, time.Second))
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)
	client := farm.NewClient(srv.URL)

	pair1 := checkpointPair("round-001-frame-0000000100-goto", []byte("state-one"), []byte(`{"k":1}`))
	pair2 := checkpointPair("round-002-frame-0000000200-goto", []byte("state-two"), []byte(`{"k":2}`))
	detail := "still on map 0x0c at (10,35)"
	finishA := farm.FinishReport{
		SaveState:     []byte("final-state-a"),
		FramePNG:      []byte("png-a"),
		RunnerVersion: "deadbeef",
		SeedBurn:      12,
		TraceTail:     []string{"enter viridian forest", "blocked at (10,35)"},
		Artifacts:     append(pair1, pair2...),
	}

	postJSON(t, srv.URL+"/v1/specs", spec("run-a"))
	failRunWithEvidence(t, client, "run-a", detail, finishA, append(pair1, pair2...))

	key, fp := failureIdentity(normalizeDetail(detail))
	link := waitIssueLink(t, w, key)
	if link.IssueNumber != e2eIssueNum || !strings.Contains(link.IssueURL, e2eIssueID) {
		t.Fatalf("link = %+v", link)
	}
	if !strings.HasPrefix(link.IssueURL, e2eUI+"/issues/") {
		t.Fatalf("issue url = %s, want UUID path under %s", link.IssueURL, e2eUI)
	}

	w.syncIssueStatuses()
	assertTriageAndDashboard(t, srv.URL, key, fp, 1)

	reports, occ, _, _, _ := ao.snapshot()
	if len(reports) != 1 || occ != 1 {
		t.Fatalf("reports=%d occ=%d, want 1/1", len(reports), occ)
	}
	got := reports[0]
	if got.Status != http.StatusCreated {
		t.Fatalf("first report HTTP %d, want 201", got.Status)
	}
	if got.Manifest.Source != "pokefarm" || got.Manifest.Fingerprint != fp {
		t.Fatalf("manifest = %+v", got.Manifest)
	}
	if got.Manifest.ExternalID != "run-a-attempt-3" || got.Manifest.ObservedRevision != "deadbeef" {
		t.Fatalf("identity = %+v", got.Manifest)
	}
	var evidence map[string]any
	if err := json.Unmarshal(got.Manifest.Evidence, &evidence); err != nil {
		t.Fatalf("evidence: %v", err)
	}
	if evidence["seed_burn"] != float64(12) || evidence["runner_version"] != "deadbeef" {
		t.Fatalf("evidence = %s", got.Manifest.Evidence)
	}
	if evidence["decision"] != "go to viridian forest" || evidence["question"] == "" {
		t.Fatalf("plan missing from evidence: %s", got.Manifest.Evidence)
	}
	for name, data := range got.Files {
		if strings.HasSuffix(name, ".gb") || strings.HasSuffix(name, ".rom") {
			t.Fatalf("ROM leaked as artifact %s", name)
		}
		sum := sha256.Sum256(data)
		if got.Hashes[name] != hex.EncodeToString(sum[:]) {
			t.Fatalf("hash mismatch for %s", name)
		}
	}
	for _, name := range []string{"final.state", "final.png", "finish-manifest.json",
		"round-001-frame-0000000100-goto.state", "round-002-frame-0000000200-goto.state"} {
		if _, ok := got.Files[name]; !ok {
			t.Fatalf("missing artifact %s in %v", name, keysOf(got.Files))
		}
	}
	if !bytes.Equal(got.Files["final.state"], []byte("final-state-a")) {
		t.Fatalf("final.state = %q", got.Files["final.state"])
	}

	w.mu.Lock()
	entry := w.outbox["run-a-attempt-3"]
	w.mu.Unlock()
	if err := w.dispatchOccurrence(entry); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	reports, occ, _, _, _ = ao.snapshot()
	if occ != 1 {
		t.Fatalf("idempotent retry created occurrence %d", occ)
	}
	if n := countCreated(reports); n != 1 {
		t.Fatalf("created reports = %d, want 1 (retry must be 200)", n)
	}

	w2 := NewWall(dumps)
	w2.SetStatePath(stateFile)
	groups := w2.triage()
	if len(groups) != 1 || groups[0].Issue == nil || groups[0].Issue.IssueID != e2eIssueID {
		t.Fatalf("reloaded triage = %+v", groups)
	}

	postJSON(t, srv.URL+"/v1/specs", spec("run-b"))
	failRunWithEvidence(t, client, "run-b", detail, farm.FinishReport{
		SaveState: []byte("final-state-b"), RunnerVersion: "deadbeef", SeedBurn: 12,
	}, nil)
	waitIssueLink(t, w, key)
	waitOccurrences(t, ao, 2)
	w.syncIssueStatuses()
	assertTriageAndDashboard(t, srv.URL, key, fp, 2)

	ao.mu.Lock()
	ao.status = "resolved"
	ao.resolution = "fixed"
	ao.fixedRev = e2eFixedSHA
	ao.mu.Unlock()
	w.syncIssueStatuses()
	w.mu.Lock()
	fixed := w.issueLinks[key]
	w.mu.Unlock()
	if fixed.Status != "resolved" || fixed.Resolution != "fixed" || fixed.FixedRevision != e2eFixedSHA {
		t.Fatalf("fixed link = %+v", fixed)
	}

	postJSON(t, srv.URL+"/v1/specs", spec("run-c"))
	failRunWithEvidence(t, client, "run-c", detail, farm.FinishReport{
		SaveState: []byte("final-state-c"), RunnerVersion: "deadbeef", SeedBurn: 12,
	}, nil)
	waitOccurrences(t, ao, 3)
	_, _, status, _, _ := ao.snapshot()
	if status != "open" {
		t.Fatalf("reopened status = %s, want open", status)
	}

	dumpPath := filepath.Join(dumps, "run-a-attempt-3.json")
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read original dump: %v", err)
	}
	var dumped farm.FinishReport
	if err := json.Unmarshal(data, &dumped); err != nil {
		t.Fatalf("decode original dump: %v", err)
	}
	if dumped.RunnerVersion != "deadbeef" || dumped.SeedBurn != 12 || !bytes.Equal(dumped.SaveState, []byte("final-state-a")) {
		t.Fatalf("dump mutated: %+v", dumped)
	}
	if len(dumped.Artifacts) != 4 {
		t.Fatalf("dump artifacts = %d, want 4 checkpoint files", len(dumped.Artifacts))
	}
}

func TestIssueHandoffCorruptEvidence(t *testing.T) {
	var calls atomic.Int32
	ao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	t.Cleanup(ao.Close)

	dir := t.TempDir()
	w := NewWall(dir)
	w.issues = newIssueClient(ao.URL, e2eProject, e2eUI, time.Second)
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	postJSON(t, srv.URL+"/v1/specs", spec("corrupt-1"))
	settleFailedRun(t, srv.URL, "corrupt-1", "blacked out", farm.FinishReport{
		SaveState: []byte("final"),
		Artifacts: []farm.Artifact{hashedNamed("round-001-frame-0000000100-goto.state", "application/octet-stream", []byte("ckpt"))},
	})
	dumpPath := filepath.Join(dir, "corrupt-1-attempt-3.json")
	raw, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	var dump farm.FinishReport
	if err := json.Unmarshal(raw, &dump); err != nil {
		t.Fatalf("decode: %v", err)
	}
	dump.Artifacts[0].SHA256 = strings.Repeat("00", 32)
	rewritten, _ := json.Marshal(dump)
	if err := os.WriteFile(dumpPath, rewritten, 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	e, ok := w.nextOutbox()
	if !ok {
		t.Fatal("no outbox")
	}
	err = w.dispatchOccurrence(e)
	if err == nil {
		t.Fatal("corrupt dispatch succeeded")
	}
	w.noteOutboxResult(e.ExternalID, err, isRetryableIssueError(err))
	if n := calls.Load(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
	if _, ok := w.nextOutbox(); ok {
		t.Fatal("permanent error was retried")
	}
	groups := w.triage()
	if len(groups) != 1 || groups[0].Outbox != outboxError {
		t.Fatalf("triage = %+v", groups)
	}
}

func TestIssueHandoffTransientRetry(t *testing.T) {
	ao := newFakeOrchestrator()
	ao.failNext = 1
	aoSrv := httptest.NewServer(ao.handler())
	t.Cleanup(aoSrv.Close)

	dumps := t.TempDir()
	stateFile := filepath.Join(t.TempDir(), "state.json")
	w := NewWall(dumps)
	w.SetStatePath(stateFile)
	w.issues = newIssueClient(aoSrv.URL, e2eProject, e2eUI, time.Second)
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	postJSON(t, srv.URL+"/v1/specs", spec("transient-1"))
	settleFailedRun(t, srv.URL, "transient-1", "still on map 0x0c at (1,2)", farm.FinishReport{SaveState: []byte("s")})
	e, ok := w.nextOutbox()
	if !ok {
		t.Fatal("no outbox")
	}
	ext := e.ExternalID
	err := w.dispatchOccurrence(e)
	if err == nil {
		t.Fatal("first dispatch should see the transient failure")
	}
	w.noteOutboxResult(e.ExternalID, err, isRetryableIssueError(err))

	w2 := NewWall(dumps)
	w2.SetStatePath(stateFile)
	w2.issues = newIssueClient(aoSrv.URL, e2eProject, e2eUI, time.Second)
	var e2 outboxEntry
	ok = false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		e2, ok = w2.nextOutbox()
		if ok {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ok {
		t.Fatal("transient failure did not survive reload")
	}
	if e2.ExternalID != ext {
		t.Fatalf("external id = %s, want %s", e2.ExternalID, ext)
	}
	if err := w2.dispatchOccurrence(e2); err != nil {
		t.Fatalf("retry after reload: %v", err)
	}
	reports, occ, _, _, _ := ao.snapshot()
	if occ != 1 || countCreated(reports) != 1 {
		t.Fatalf("reports=%d created=%d occ=%d, want one occurrence", len(reports), countCreated(reports), occ)
	}
	if reports[len(reports)-1].Manifest.ExternalID != ext {
		t.Fatalf("retried a different external id: %s vs %s", reports[len(reports)-1].Manifest.ExternalID, ext)
	}
}

func failRunWithEvidence(t *testing.T, client *farm.Client, runID, detail string, extra farm.FinishReport, checkpoints []farm.Artifact) {
	t.Helper()
	ctx := context.Background()
	for i := 1; i <= maxAttempts; i++ {
		got, err := client.Lease(ctx)
		if err != nil || got == nil {
			t.Fatalf("lease attempt %d: %v %+v", i, err, got)
		}
		report := extra
		report.RunID = runID
		report.Attempt = i
		report.Reason = "error"
		report.Detail = detail
		if i < maxAttempts {
			report.Artifacts = nil
			report.SaveState = []byte("retry")
			if err := client.Finish(ctx, report); err != nil {
				t.Fatalf("finish attempt %d: %v", i, err)
			}
			continue
		}
		_, err = client.Heartbeat(ctx, farm.Heartbeat{
			RunID:    runID,
			Frame:    18000,
			Map:      0x33,
			X:        15,
			Y:        13,
			Trace:    "viridian forest",
			Question: "1: go to viridian forest",
			Decision: "go to viridian forest",
		})
		if err != nil {
			t.Fatalf("heartbeat: %v", err)
		}
		if len(checkpoints) > 0 {
			if err := client.Checkpoint(ctx, farm.CheckpointReport{RunID: runID, Attempt: i, Artifacts: checkpoints}); err != nil {
				t.Fatalf("checkpoint: %v", err)
			}
		}
		if err := client.Finish(ctx, report); err != nil {
			t.Fatalf("finish attempt %d: %v", i, err)
		}
	}
}

func waitIssueLink(t *testing.T, w *Wall, key string) IssueLink {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		link, ok := w.issueLinks[key]
		w.mu.Unlock()
		if ok && link.IssueID != "" {
			return link
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for issue link %s", key)
	return IssueLink{}
}

func waitOccurrences(t *testing.T, ao *fakeOrchestrator, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, occ, _, _, _ := ao.snapshot()
		if occ >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, occ, _, _, _ := ao.snapshot()
	t.Fatalf("occurrences = %d, want %d", occ, want)
}

func assertTriageAndDashboard(t *testing.T, base, key, fp string, occ int64) {
	t.Helper()
	res, err := http.Get(base + "/v1/triage")
	if err != nil {
		t.Fatalf("GET triage: %v", err)
	}
	defer res.Body.Close()
	var groups []triageGroup
	if err := json.NewDecoder(res.Body).Decode(&groups); err != nil {
		t.Fatalf("decode triage: %v", err)
	}
	if len(groups) != 1 || groups[0].Key != key || groups[0].Fingerprint != fp {
		t.Fatalf("triage = %+v", groups)
	}
	if groups[0].Issue == nil || groups[0].Issue.IssueNumber != e2eIssueNum {
		t.Fatalf("triage missing #42: %+v", groups[0].Issue)
	}
	if occ > 0 && groups[0].Issue.OccurrenceCount != occ {
		t.Fatalf("triage occurrence_count = %d, want %d", groups[0].Issue.OccurrenceCount, occ)
	}
	if !strings.Contains(groups[0].Issue.IssueURL, e2eIssueID) {
		t.Fatalf("triage url = %s", groups[0].Issue.IssueURL)
	}
	if strings.Contains(groups[0].Issue.IssueURL, "/issues/42") {
		t.Fatal("triage used issue number in the URL")
	}

	res, err = http.Get(base + "/v1/dashboard")
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	defer res.Body.Close()
	var dash struct {
		Runs []struct {
			RunID string     `json:"run_id"`
			Issue *IssueLink `json:"issue"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(res.Body).Decode(&dash); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	var found *IssueLink
	for _, r := range dash.Runs {
		if r.Issue != nil {
			found = r.Issue
			break
		}
	}
	if found == nil || found.IssueNumber != e2eIssueNum || !strings.Contains(found.IssueURL, e2eIssueID) {
		t.Fatalf("dashboard issue = %+v", found)
	}
}

func checkpointPair(base string, state, knowledge []byte) []farm.Artifact {
	return []farm.Artifact{
		hashedNamed(base+".state", "application/octet-stream", state),
		hashedNamed(base+".knowledge-v4.json", "application/json", knowledge),
	}
}

func countCreated(reports []capturedIssueReport) int {
	n := 0
	for _, r := range reports {
		if r.Status == http.StatusCreated {
			n++
		}
	}
	return n
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
