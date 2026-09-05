package farm

import (
	"strings"
	"testing"
)

func TestValidateFinishArtifactsAcceptsS3Reference(t *testing.T) {
	report := FinishReport{Artifacts: []Artifact{{
		Name:      "run.gbrun",
		MediaType: "application/octet-stream",
		SHA256:    strings.Repeat("a", 64),
		Store:     ArtifactStoreS3,
		Bucket:    "pokepilot",
		ObjectKey: "runs/run-42/attempt-1/run.gbrun",
		Size:      3 << 20,
	}}}
	if err := ValidateFinishArtifacts(report); err != nil {
		t.Fatalf("valid remote artifact: %v", err)
	}
}

func TestValidateFinishArtifactsRejectsRemoteArtifactWithInlineData(t *testing.T) {
	report := FinishReport{Artifacts: []Artifact{{
		Name:      "run.gbrun",
		MediaType: "application/octet-stream",
		SHA256:    strings.Repeat("a", 64),
		Data:      []byte("must not be inline"),
		Store:     ArtifactStoreS3,
		Bucket:    "pokepilot",
		ObjectKey: "runs/run-42/attempt-1/run.gbrun",
		Size:      1024,
	}}}
	if err := ValidateFinishArtifacts(report); err == nil {
		t.Fatal("remote artifact with inline data must be rejected")
	}
}

func TestRemoteArtifactDoesNotConsumeInlineBudget(t *testing.T) {
	report := FinishReport{Artifacts: []Artifact{{
		Name:      "run.gbrun",
		MediaType: "application/octet-stream",
		SHA256:    strings.Repeat("b", 64),
		Store:     ArtifactStoreS3,
		Bucket:    "pokepilot",
		ObjectKey: "runs/run-42/attempt-1/run.gbrun",
		Size:      int64(MaxFinishArtifactBytes) * 10,
	}}}
	if err := ValidateFinishArtifacts(report); err != nil {
		t.Fatalf("remote size must not count toward inline limit: %v", err)
	}
}
