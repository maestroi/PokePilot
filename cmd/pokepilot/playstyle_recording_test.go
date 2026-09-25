package main

import (
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestFarmRecordingMetadataIncludesPlayStyle(t *testing.T) {
	spec := farm.Spec{RunID: "style-recording", Attempt: 1, LLMProfile: "auto", PlayStyle: "team_builder", Purpose: farm.RunPurposeDebugCoverage}
	got := farmRecordingMetadata(spec, "llm", "squirtle", "", "badges:1", 0, "test")
	if got["play_style"] != "team_builder" {
		t.Fatalf("play_style metadata = %q, want team_builder", got["play_style"])
	}
	if got["purpose"] != "debug_coverage" {
		t.Fatalf("purpose metadata = %q, want debug_coverage", got["purpose"])
	}
	if got["llm_profile"] != "auto" {
		t.Fatalf("llm_profile metadata = %q, want auto", got["llm_profile"])
	}
}
