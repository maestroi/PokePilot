package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestLoadOccurrenceEvidenceFallsBackToLegacyRetryDump(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	const runID = "run-legacy-retry"

	attachment := hashedNamed("trace.txt", "text/plain", []byte("attached evidence"))
	report := farm.FinishReport{
		RunID:     runID,
		Reason:    "error",
		Detail:    "terminal retry failure",
		Artifacts: []farm.Artifact{attachment},
		// Attempt is intentionally zero: legacy/hand-built runners may omit it.
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, safeDumpName(runID)), data, 0o644); err != nil {
		t.Fatal(err)
	}

	dump, artifacts, err := w.loadOccurrenceEvidence(outboxEntry{RunID: runID, Attempt: 3})
	if err != nil {
		t.Fatalf("load legacy retry evidence: %v", err)
	}
	if dump.Attempt != 3 {
		t.Fatalf("normalized attempt = %d, want 3", dump.Attempt)
	}
	found := false
	for _, artifact := range artifacts {
		if artifact.Name == attachment.Name {
			found = true
			if string(artifact.Data) != string(attachment.Data) {
				t.Fatalf("attachment data = %q, want %q", artifact.Data, attachment.Data)
			}
		}
	}
	if !found {
		t.Fatalf("attachment %q missing from issue evidence: %+v", attachment.Name, artifacts)
	}
}

func TestLoadOccurrenceEvidenceDoesNotReuseWrongLegacyAttempt(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	const runID = "run-explicit-attempt"

	report := farm.FinishReport{
		RunID:   runID,
		Attempt: 1,
		Reason:  "error",
		Detail:  "first attempt failure",
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, safeDumpName(runID)), data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err = w.loadOccurrenceEvidence(outboxEntry{RunID: runID, Attempt: 3})
	if err == nil {
		t.Fatal("expected mismatched legacy attempt to be rejected")
	}
	if !strings.Contains(err.Error(), "belongs to attempt 1, wanted 3") {
		t.Fatalf("unexpected error: %v", err)
	}
}
