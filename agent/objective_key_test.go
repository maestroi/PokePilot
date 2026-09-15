package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestObjectiveKeyIgnoresPresentationNote(t *testing.T) {
	base := Objective{Kind: KindGoTo, Place: "route 1", Flee: true, Intent: "explore"}
	decorated := base
	decorated.Note = "(different UI copy that must never change durable identity)"
	if base.Key() != decorated.Key() || objectiveStorageKey(base) != objectiveStorageKey(decorated) {
		t.Fatalf("presentation note changed objective identity: base=%+v decorated=%+v", base.Key(), decorated.Key())
	}

	changed := base
	changed.Flee = false
	if base.Key() == changed.Key() {
		t.Fatal("execution-relevant flee policy did not change objective identity")
	}
	changed = base
	changed.Intent = "dex-static"
	if base.Key() == changed.Key() {
		t.Fatal("execution-relevant intent did not change objective identity")
	}
}

func TestPlanStepKeySurvivesDisplayWordingChange(t *testing.T) {
	o := Objective{Kind: KindGoTo, Place: "route 1", Flee: true}
	plan := Plan{
		Goal:     "continue north",
		Steps:    []string{"OLD COPY THAT NO LONGER MATCHES Objective.String"},
		StepKeys: []ObjectiveKey{o.Key()},
	}
	got, skipped, ok := resolvePlanStep(&plan, []Objective{o})
	if !ok || skipped != 0 || got.Key() != o.Key() {
		t.Fatalf("semantic resume = (%+v,%d,%v), want objective without a display-string dependency", got, skipped, ok)
	}
}

func TestKnowledgeWritesCanonicalObjectiveIdentity(t *testing.T) {
	known := NewKnowledge(nil)
	o := Objective{Kind: KindTrain, Species: "pikachu", Level: 12, Slot: 1, Note: "planner copy"}
	known.Done(o)
	if got := known.Completed[objectiveStorageKey(o)]; got != 1 {
		t.Fatalf("canonical completion count = %d, want 1", got)
	}
	if _, legacy := known.Completed[o.String()]; legacy {
		t.Fatalf("completion still keyed by display text %q", o.String())
	}
}

func TestLoadCheckpointMemoryMigratesV4PlanAndKnowledge(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-1.state")
	if err := os.WriteFile(statePath, []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"version":   4,
		"visited":   []uint8{1},
		"places":    []string{"route 1"},
		"completed": []map[string]any{{"Objective": "go to route 1", "Times": 2}},
		"talked":    []any{},
		"failures":  []map[string]any{{"Objective": "train the lead to level 12", "Times": 1, "Last": "target not reached"}},
		"plan": map[string]any{
			"goal":  "old plan",
			"steps": []string{"go to route 1"},
			"step":  0,
		},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := knowledgePathForStateVersion(statePath, legacyPresentationVersion)
	if err := os.WriteFile(legacyPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got := LoadCheckpointMemory(statePath, nil, nil)
	if got.Plan.Goal != "old plan" || len(got.Plan.Steps) != 1 || len(got.Plan.StepKeys) != 0 {
		t.Fatalf("migrated legacy plan = %+v", got.Plan)
	}
	if got.Knowledge.Completed["go to route 1"] != 2 {
		t.Fatalf("legacy completion did not survive migration: %+v", got.Knowledge.Completed)
	}
	if got.Knowledge.Failures["train the lead to level 12"].Times != 1 {
		t.Fatalf("legacy failure did not survive migration: %+v", got.Knowledge.Failures)
	}
}
