package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

func TestVerifyPortableBundleWithoutFailureContractIsExplicit(t *testing.T) {
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "safe", "repro-result.json")
	mat := portableMaterialized{
		Manifest: farm.PortableReproManifest{
			Version:          farm.PortableReproVersion,
			IssueNumber:      465,
			RunID:            "run-stuck",
			ObservedRevision: "old-revision",
			Fingerprint:      "sha256:legacy",
			Objective:        "make progress toward run goal",
		},
		Dir: dir,
	}
	got, err := verifyPortableBundle(mat, resultPath)
	if err != nil {
		t.Fatalf("verifyPortableBundle: %v", err)
	}
	if got.Classification != verdictContractUnavailable {
		t.Fatalf("classification=%q want=%q", got.Classification, verdictContractUnavailable)
	}
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var persisted portableReproVerdict
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Classification != verdictContractUnavailable || persisted.Issue != 465 {
		t.Fatalf("persisted=%+v", persisted)
	}
}

func TestObjectiveFromFailure(t *testing.T) {
	got, err := objectiveFromFailure(farm.FailureObjective{
		Kind:    "go_to",
		Place:   "route 9",
		Flee:    true,
		Species: "pikachu",
		Slot:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != agent.KindGoTo || got.Place != "route 9" || !got.Flee {
		t.Fatalf("objective=%+v", got)
	}
}

func TestObjectiveFromFailureRejectsUnknownKind(t *testing.T) {
	if _, err := objectiveFromFailure(farm.FailureObjective{Kind: "mystery"}); err == nil {
		t.Fatal("expected unsupported objective kind error")
	}
}
