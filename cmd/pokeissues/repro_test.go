package main

import (
	"strings"
	"testing"
)

func TestFindReproCheckpointPrefersLatestPairedStateAndKnowledgeVersion(t *testing.T) {
	artifacts := []artifactMeta{
		{Name: "round-065-frame-000001-old.state", SHA256: "state65"},
		{Name: "round-065-frame-000001-old.knowledge-v4.json", SHA256: "knowledge65"},
		{Name: "round-066-frame-000002-fail.state", SHA256: "state66"},
		{Name: "round-066-frame-000002-fail.knowledge-v3.json", SHA256: "knowledge-v3"},
		{Name: "round-066-frame-000002-fail.knowledge-v4.json", SHA256: "knowledge-v4"},
		{Name: "final.state", SHA256: "final"},
	}

	got, ok := findReproCheckpoint(artifacts)
	if !ok {
		t.Fatal("expected replayable checkpoint")
	}
	if got.State.Name != "round-066-frame-000002-fail.state" {
		t.Fatalf("state = %q", got.State.Name)
	}
	if got.Knowledge.Name != "round-066-frame-000002-fail.knowledge-v4.json" {
		t.Fatalf("knowledge = %q", got.Knowledge.Name)
	}
}

func TestFindReproCheckpointRejectsUnpairedState(t *testing.T) {
	if _, ok := findReproCheckpoint([]artifactMeta{{Name: "round-066-frame-000002-fail.state"}}); ok {
		t.Fatal("unpaired state must not be advertised as reproducible")
	}
}

func TestIssueBodyIncludesPinnedPokereproCommand(t *testing.T) {
	manifest := sampleManifest("run-42-attempt-3-objective-key")
	artifacts := []artifactMeta{
		{Name: "round-066-frame-000002-fail.state", SHA256: "state-sha"},
		{Name: "round-066-frame-000002-fail.knowledge-v4.json", SHA256: "knowledge-sha"},
	}
	body := renderIssueBody("https://pokemon.test", manifest, artifacts)

	for _, want := range []string{
		"## Reproduce",
		"**Checkpoint:** `round-066-frame-000002-fail.state`",
		"**Knowledge:** `round-066-frame-000002-fail.knowledge-v4.json`",
		"**State SHA-256:** `state-sha`",
		"**Knowledge SHA-256:** `knowledge-sha`",
		"go run ./cmd/pokerepro -wall 'https://pokemon.test' -run 'run-42' -attempt 3 -checkpoint 'round-066-frame-000002-fail.state' -play",
		"authenticated Run Inspector API",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("issue body missing %q:\n%s", want, body)
		}
	}
}
