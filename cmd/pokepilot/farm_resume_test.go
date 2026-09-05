package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestMaterializeFarmResumeKeepsStateAndKnowledgePaired(t *testing.T) {
	dir := t.TempDir()
	state := runnerResumeArtifact("round-007-frame-0000000700-goto.state", []byte("emulator-state"), "application/octet-stream")
	knowledge := runnerResumeArtifact("round-007-frame-0000000700-goto.knowledge-v4.json", []byte(`{"intent":"recover from pewter"}`), "application/json")
	cp := farm.ResumeCheckpoint{Attempt: 1, State: state, Knowledge: &knowledge}
	if err := materializeFarmResume(dir, cp); err != nil {
		t.Fatal(err)
	}
	if got := farmResumePath(dir); got != filepath.Join(dir, state.Name) {
		t.Fatalf("resume path = %q", got)
	}
	if got, err := os.ReadFile(filepath.Join(dir, knowledge.Name)); err != nil || string(got) != string(knowledge.Data) {
		t.Fatalf("knowledge = %q, %v", got, err)
	}
}

func TestMaterializeFarmResumeRejectsStateWithoutKnowledge(t *testing.T) {
	state := runnerResumeArtifact("round-001-frame-0000000100-goto.state", []byte("state"), "application/octet-stream")
	if err := materializeFarmResume(t.TempDir(), farm.ResumeCheckpoint{State: state}); err == nil {
		t.Fatal("missing knowledge should reject LLM resume")
	}
}

func TestRunFarmLLMWiresResumeIntoAgentBudget(t *testing.T) {
	src, err := os.ReadFile("farm.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "resumeFrom := farmResumePath(checkpointDir)") {
		t.Fatal("runFarmLLM does not resolve the durable resume marker")
	}
	if !strings.Contains(text, "ResumeFrom:    resumeFrom") {
		t.Fatal("runFarmLLM does not pass ResumeFrom into agent.Budget")
	}
	if !strings.Contains(text, `starter != "" && resumeFrom == ""`) {
		t.Fatal("resumed LLM run would replay starter acquisition")
	}
}

func runnerResumeArtifact(name string, data []byte, mediaType string) farm.Artifact {
	sum := sha256.Sum256(data)
	return farm.Artifact{Name: name, MediaType: mediaType, SHA256: hex.EncodeToString(sum[:]), Data: data}
}
