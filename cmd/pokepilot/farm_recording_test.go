package main

import (
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestFarmRecordingMetadata(t *testing.T) {
	spec := farm.Spec{
		RunID:      "run-42",
		Attempt:    3,
		Seed:       12345,
		LLMProfile: "primary",
	}
	got := farmRecordingMetadata(spec, "llm", "squirtle", "", "earn the Boulder Badge", 87, "abc123")
	want := map[string]string{
		"run_id":         "run-42",
		"attempt":        "3",
		"runner_version": "abc123",
		"planner":        "llm",
		"seed":           "12345",
		"seed_burn":      "87",
		"starter":        "squirtle",
		"goal":           "earn the Boulder Badge",
		"llm_profile":    "primary",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata = %#v, want %#v", got, want)
	}
	if _, ok := got["dest"]; ok {
		t.Fatal("empty destination must be omitted")
	}
}

func TestFarmRecordingArtifactValidates(t *testing.T) {
	data := []byte("deterministic gbrun bytes")
	art := farmRecordingArtifact(data)
	if art.Name != farmRecordingName {
		t.Fatalf("artifact name = %q, want %q", art.Name, farmRecordingName)
	}
	if art.MediaType != "application/octet-stream" {
		t.Fatalf("media type = %q", art.MediaType)
	}
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: []farm.Artifact{art}}); err != nil {
		t.Fatalf("recording artifact should pass farm validation: %v", err)
	}
}
