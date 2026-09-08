package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointMemoryRestoresPlanAdditively(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-001.state")
	if err := os.WriteFile(statePath, []byte("state-not-read-here"), 0o644); err != nil {
		t.Fatal(err)
	}
	k := NewKnowledge(map[uint8][]uint8{})
	plan := Plan{Goal: "reach pewter", Steps: []string{"go to route 1", "go to viridian city"}, Step: 1, Round: 7}
	if err := writeMemoryFile(statePath, k, "", 0, plan); err != nil {
		t.Fatal(err)
	}
	got := LoadCheckpointMemory(statePath, map[uint8][]uint8{}, nil)
	if got.Plan.Goal != plan.Goal || got.Plan.Step != 1 || len(got.Plan.Steps) != 2 {
		t.Fatalf("restored plan = %+v, want %+v", got.Plan, plan)
	}
}

func TestCheckpointMemoryVersionFourWithoutPlanLoadsEmptyPlan(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-001.state")
	if err := os.WriteFile(statePath, []byte("state-not-read-here"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"version":   memoryVersion,
		"visited":   []uint8{},
		"places":    []string{},
		"completed": []Completion{},
		"talked":    []talkedKey{},
	}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(knowledgePathForState(statePath), data, 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadCheckpointMemory(statePath, map[uint8][]uint8{}, nil)
	if got.Plan.Goal != "" || len(got.Plan.Steps) != 0 || got.Plan.Step != 0 {
		t.Fatalf("legacy checkpoint restored non-empty plan: %+v", got.Plan)
	}
}
