package main

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestObjectiveFailureEvidenceSkipsRemoteRecordingAttachment(t *testing.T) {
	dump := farm.FinishReport{Artifacts: []farm.Artifact{{
		Name:      "run.gbrun",
		MediaType: "application/octet-stream",
		SHA256:    strings.Repeat("a", 64),
		Store:     farm.ArtifactStoreS3,
		Bucket:    "pokepilot",
		ObjectKey: "runs/run-42/attempt-1/run.gbrun",
		Size:      1024,
	}}}
	got := objectiveFailureEvidenceArtifacts(dump, farm.ObjectiveFailure{LastRound: 12, Blocking: true})
	for _, a := range got {
		if a.Name == "run.gbrun" {
			t.Fatalf("remote run.gbrun must not be emitted as a multipart attachment: %+v", a)
		}
	}
}
