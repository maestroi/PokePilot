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

func TestClassifyIssueOccurrence(t *testing.T) {
	tests := []struct {
		name string
		link IssueLink
		want issueOccurrenceDisposition
	}{
		{name: "new fingerprint", want: occurrenceReport},
		{name: "open", link: IssueLink{IssueID: "id", Status: "open"}, want: occurrenceQuarantine},
		{name: "investigating", link: IssueLink{IssueID: "id", Status: "investigating"}, want: occurrenceQuarantine},
		{name: "reopened beats stale fixed resolution", link: IssueLink{IssueID: "id", Status: "reopened", Resolution: "fixed"}, want: occurrenceQuarantine},
		{name: "fixed is regression", link: IssueLink{IssueID: "id", Status: "resolved", Resolution: "fixed", FixedRevision: "old"}, want: occurrenceRegression},
		{name: "closed unknown resolution is conservatively regression", link: IssueLink{IssueID: "id", Status: "closed"}, want: occurrenceRegression},
		{name: "ignored stays quarantined", link: IssueLink{IssueID: "id", Status: "resolved", Resolution: "ignored"}, want: occurrenceQuarantine},
		{name: "unknown lifecycle is reported", link: IssueLink{IssueID: "id", Status: "future-state"}, want: occurrenceReport},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyIssueOccurrence(tc.link); got != tc.want {
				t.Fatalf("disposition = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStructuredObjectiveFailureQuarantinesActiveIssue(t *testing.T) {
	var calls atomic.Int32
	ao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "quarantined occurrence must not reach orchestrator", http.StatusInternalServerError)
	}))
	t.Cleanup(ao.Close)

	w := NewWall(t.TempDir())
	w.issues = newIssueClient(ao.URL, "p", "http://ui", time.Second)
	failure, occurrence := structuredObjectiveFailureFixture(t, "build-new")
	w.mu.Lock()
	w.issueLinks[occurrence.Key] = IssueLink{
		IssueID:      "issue-1",
		IssueNumber:  42,
		IssueURL:     "http://ui/issues/issue-1",
		Status:       "open",
		Fingerprint:  occurrence.Fingerprint,
		OccurrenceCount: 1,
	}
	w.mu.Unlock()

	dump := farm.FinishReport{RunID: "run-q", Attempt: 1, Reason: "error", RunnerVersion: "build-new"}
	if err := w.reportObjectiveFailure(dump, failure); err != nil {
		t.Fatalf("reportObjectiveFailure: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("orchestrator calls = %d, want 0", calls.Load())
	}
	ext := objectiveFailureExternalID(dump.RunID, dump.Attempt, occurrence.Key)
	w.mu.Lock()
	entry := w.outbox[ext]
	link := w.issueLinks[occurrence.Key]
	w.mu.Unlock()
	if entry.Status != outboxQuarantined || !strings.Contains(entry.Note, "equivalent structured fingerprint") {
		t.Fatalf("quarantine outbox = %+v", entry)
	}
	if link.QuarantinedCount != 1 || link.LastObservedRun != dump.RunID || link.LastObservedRevision != "build-new" || link.LastDisposition != string(occurrenceQuarantine) {
		t.Fatalf("quarantine link = %+v", link)
	}

	// Restart rescans / duplicate calls for the same external occurrence are
	// idempotent and must not inflate the quarantine count.
	if err := w.reportObjectiveFailure(dump, failure); err != nil {
		t.Fatalf("idempotent reportObjectiveFailure: %v", err)
	}
	w.mu.Lock()
	link = w.issueLinks[occurrence.Key]
	w.mu.Unlock()
	if link.QuarantinedCount != 1 {
		t.Fatalf("quarantined count = %d, want 1 after duplicate", link.QuarantinedCount)
	}
}

func TestStructuredObjectiveFailureReportsFixedRegression(t *testing.T) {
	var calls atomic.Int32
	var gotManifest issueReportManifest
	ao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		manifest, err := decodeIssueManifest(r)
		if err != nil {
			t.Errorf("decode manifest: %v", err)
			http.Error(w, "bad multipart", http.StatusBadRequest)
			return
		}
		gotManifest = manifest
		json.NewEncoder(w).Encode(map[string]any{
			"issue":      map[string]any{"id": "issue-1", "issue_number": 42, "status": "open"},
			"occurrence": map[string]any{"id": "occ-regression", "external_id": manifest.ExternalID},
			"automation": map[string]any{"status": "captured"},
		})
	}))
	t.Cleanup(ao.Close)

	w := NewWall(t.TempDir())
	w.issues = newIssueClient(ao.URL, "p", "http://ui", time.Second)
	failure, occurrence := structuredObjectiveFailureFixture(t, "build-new")
	w.mu.Lock()
	w.issueLinks[occurrence.Key] = IssueLink{
		IssueID:        "issue-1",
		IssueNumber:    42,
		IssueURL:       "http://ui/issues/issue-1",
		Status:         "resolved",
		Resolution:     "fixed",
		FixedRevision:  "build-fixed",
		Fingerprint:    occurrence.Fingerprint,
	}
	w.mu.Unlock()

	dump := farm.FinishReport{RunID: "run-regression", Attempt: 2, Reason: "error", RunnerVersion: "build-new"}
	if err := w.reportObjectiveFailure(dump, failure); err != nil {
		t.Fatalf("reportObjectiveFailure: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("orchestrator calls = %d, want 1 regression report", calls.Load())
	}
	if gotManifest.Fingerprint != occurrence.Fingerprint || gotManifest.ObservedRevision != "build-new" || gotManifest.Severity != "critical" || !strings.Contains(gotManifest.Title, "[farm][regression]") {
		t.Fatalf("regression manifest = %+v", gotManifest)
	}
	var evidence map[string]any
	if err := json.Unmarshal(gotManifest.Evidence, &evidence); err != nil {
		t.Fatalf("evidence: %v", err)
	}
	if evidence["classification"] != "regression" || evidence["prior_fixed_revision"] != "build-fixed" || evidence["fingerprint"] != occurrence.Fingerprint {
		t.Fatalf("regression evidence = %s", gotManifest.Evidence)
	}

	ext := objectiveFailureExternalID(dump.RunID, dump.Attempt, occurrence.Key)
	w.mu.Lock()
	entry := w.outbox[ext]
	link := w.issueLinks[occurrence.Key]
	w.mu.Unlock()
	if entry.Status != outboxComplete {
		t.Fatalf("regression outbox = %+v", entry)
	}
	if link.Status != "open" || link.LastDisposition != string(occurrenceRegression) || link.LastObservedRevision != "build-new" {
		t.Fatalf("reopened link = %+v", link)
	}
}

func TestStructuredRunOutboxDefersToObjectiveReporter(t *testing.T) {
	var calls atomic.Int32
	ao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		manifest, err := decodeIssueManifest(r)
		if err != nil {
			http.Error(w, "bad multipart", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"issue":      map[string]any{"id": "issue-structured", "issue_number": 7, "status": "open"},
			"occurrence": map[string]any{"id": "occ", "external_id": manifest.ExternalID},
			"automation": map[string]any{"status": "captured"},
		})
	}))
	t.Cleanup(ao.Close)

	dir := t.TempDir()
	w := NewWall(dir)
	w.issues = newIssueClient(ao.URL, "p", "http://ui", time.Second)
	failure, occurrence := structuredObjectiveFailureFixture(t, "build-a")
	artifact, err := farm.NewObjectiveFailureArtifact([]farm.ObjectiveFailure{failure})
	if err != nil {
		t.Fatalf("NewObjectiveFailureArtifact: %v", err)
	}
	dump := farm.FinishReport{
		RunID:         "run-structured",
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
		ExternalID: "run-structured-attempt-1",
		RunID:      dump.RunID,
		Attempt:    1,
		Key:        occurrence.Key,
		Status:     outboxPending,
	}
	w.mu.Lock()
	w.outbox[general.ExternalID] = general
	w.mu.Unlock()

	if err := w.dispatchOccurrence(general); err != nil {
		t.Fatalf("dispatchOccurrence: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("generic run handoff called orchestrator %d time(s), want 0", calls.Load())
	}
	w.mu.Lock()
	deferred := w.outbox[general.ExternalID]
	w.mu.Unlock()
	if deferred.Status != outboxQuarantined || !strings.Contains(deferred.Note, "structured objective failure reporter") {
		t.Fatalf("generic structured outbox = %+v", deferred)
	}

	if err := w.reportObjectiveFailure(dump, failure); err != nil {
		t.Fatalf("structured reporter: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("structured reporter calls = %d, want exactly 1", calls.Load())
	}
}

func structuredObjectiveFailureFixture(t *testing.T, build string) (farm.ObjectiveFailure, farm.FailureOccurrence) {
	t.Helper()
	identity := farm.FailureIdentity{
		Version: farm.FailureIdentityVersion,
		Game:    "pokemon",
		Adapter: "pokemon-red",
		Objective: farm.FailureObjective{
			Kind:  "go_to",
			Place: "route_12",
			Flee:  true,
		},
		Outcome:      "blocked",
		Cause:        "route_prerequisite_missing",
		CauseContext: []string{"can_clear_snorlax"},
		Initial: farm.FailureState{
			Location:     "route_12",
			X:            10,
			Y:            61,
			Controllable: true,
		},
		Final: farm.FailureState{
			Location:     "route_12",
			X:            10,
			Y:            61,
			Controllable: true,
		},
	}
	occurrence, err := farm.NewFailureOccurrence(identity, build, 4, "round-004-frame-0000000400-goto.state", "blocked by snorlax", time.Unix(100, 0))
	if err != nil {
		t.Fatalf("NewFailureOccurrence: %v", err)
	}
	failure := farm.ObjectiveFailure{
		Objective:    "go to route 12, fleeing wild battles",
		Error:        "route prerequisite missing",
		Count:        2,
		FirstRound:   3,
		LastRound:    4,
		Map:          0x17,
		X:            10,
		Y:            61,
		Blocking:     true,
		ObservedAt:   occurrence.ObservedAt,
		Key:          occurrence.Key,
		Fingerprint:  occurrence.Fingerprint,
		Identity:     &occurrence.Identity,
		Build:        build,
		Outcome:      occurrence.Identity.Outcome,
		Cause:        occurrence.Identity.Cause,
		CauseContext: append([]string(nil), occurrence.Identity.CauseContext...),
		Checkpoint:   occurrence.Checkpoint,
	}
	return failure, occurrence
}

func decodeIssueManifest(r *http.Request) (issueReportManifest, error) {
	var out issueReportManifest
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return out, err
	}
	if media != "multipart/form-data" {
		return out, &unexpectedMediaType{got: media}
	}
	reader := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if part.FormName() != "report" {
			continue
		}
		data, err := io.ReadAll(part)
		if err != nil {
			return out, err
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return out, err
		}
	}
}

type unexpectedMediaType struct{ got string }

func (e *unexpectedMediaType) Error() string { return "unexpected media type: " + e.got }
