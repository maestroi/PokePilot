package main

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestReportObjectiveFailureSendsCriticalProgressionBlocker(t *testing.T) {
	var got issueReportManifest
	ao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "multipart/form-data" {
			t.Fatalf("content type = %q: %v", r.Header.Get("Content-Type"), err)
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("part: %v", err)
			}
			body, _ := io.ReadAll(part)
			if part.FormName() == "report" {
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("manifest: %v", err)
				}
			}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"issue":      map[string]any{"id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "issue_number": 91, "status": "open"},
			"occurrence": map[string]any{"id": "occ-1", "external_id": got.ExternalID},
			"automation": map[string]any{"status": "investigation_started"},
		})
	}))
	t.Cleanup(ao.Close)

	failure := farm.ObjectiveFailure{
		Objective:  "go to mt moon b1f, fleeing wild battles",
		Error:      "skill: step left blocked at (10,22)",
		Count:      2,
		FirstRound: 14,
		LastRound:  15,
		Map:        0x3b,
		X:          10,
		Y:          22,
		Blocking:   true,
	}
	telemetry, err := farm.NewObjectiveFailureArtifact([]farm.ObjectiveFailure{failure})
	if err != nil {
		t.Fatalf("telemetry: %v", err)
	}
	dump := farm.FinishReport{
		RunID:         "run-moon",
		Attempt:       1,
		Reason:        "failed",
		RunnerVersion: "deadbeef",
		SaveState:     []byte("final-state"),
		Artifacts:     []farm.Artifact{telemetry},
		ProgressFinal: &farm.Progress{Round: 15, Badges: 1, Maps: 12, Map: 0x3b, MapName: "Mt. Moon 1F"},
	}

	w := NewWall(t.TempDir())
	w.issues = newIssueClient(ao.URL, "proj", "http://ui", time.Second)
	if err := w.reportObjectiveFailure(dump, failure); err != nil {
		t.Fatalf("reportObjectiveFailure: %v", err)
	}

	if got.Severity != "critical" {
		t.Fatalf("severity = %q, want critical", got.Severity)
	}
	if !strings.Contains(got.Title, "progression-blocker") || !strings.Contains(got.Summary, "no later major progress") {
		t.Fatalf("title/summary = %q / %q", got.Title, got.Summary)
	}
	var evidence map[string]any
	if err := json.Unmarshal(got.Evidence, &evidence); err != nil {
		t.Fatalf("evidence: %v", err)
	}
	if evidence["classification"] != "progression-blocker" || evidence["map"] != "0x3b" {
		t.Fatalf("evidence = %+v", evidence)
	}

	key, _ := failureIdentity(objectiveFailurePattern(failure))
	ext := objectiveFailureExternalID(dump.RunID, dump.Attempt, key)
	w.mu.Lock()
	link := w.issueLinks[key]
	entry := w.outbox[ext]
	w.mu.Unlock()
	if link.IssueNumber != 91 || entry.Status != outboxComplete {
		t.Fatalf("link=%+v outbox=%+v", link, entry)
	}
}

func TestObjectiveFailureIdentityKeepsMapsDistinct(t *testing.T) {
	a := farm.ObjectiveFailure{Objective: "go somewhere", Error: "skill: no path at (10,22)", Map: 0x3b}
	b := a
	b.Map = 0x0c
	ka, _ := failureIdentity(objectiveFailurePattern(a))
	kb, _ := failureIdentity(objectiveFailurePattern(b))
	if ka == kb {
		t.Fatalf("map-local failures collapsed to one key %s", ka)
	}
}
