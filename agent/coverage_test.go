package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoverageTracksCompletionistBreadth(t *testing.T) {
	coverage := newCoverageTracker()
	known := NewKnowledge(nil)
	known.SawLocation("route-1")
	known.SawLocation("viridian-city")
	known.TalkedAt("viridian-city", 4, 5)

	initial := Observation{
		Bag: []Item{
			{Name: "potion", Quantity: 1},
			{Name: "tm01", Quantity: 1},
		},
		PokedexOwned: []SpeciesID{"squirtle", "pidgey"},
		PokedexSeen:  []SpeciesID{"squirtle", "pidgey", "rattata"},
		Party: []PartyMon{
			{Species: "squirtle", Level: 12},
			{Species: "pidgey", Level: 17},
		},
	}
	coverage.seed(initial)
	coverage.noteSuccess(Objective{Kind: KindTrainer, Location: "route-1", X: 8, Y: 4}, initial, initial)
	coverage.noteSuccess(Objective{Kind: KindUseItem, Item: "tm01", Slot: 0}, initial, initial)

	afterCatch := initial
	afterCatch.PokedexOwned = []SpeciesID{"squirtle", "pidgey", "rattata"}
	coverage.noteSuccess(Objective{Kind: KindCatch, Species: "rattata"}, initial, afterCatch)

	afterEvolution := afterCatch
	afterEvolution.Party = append([]PartyMon(nil), afterCatch.Party...)
	afterEvolution.Party[1].Species = "pidgeotto"
	afterEvolution.Party[1].Level = 18
	afterEvolution.PokedexOwned = []SpeciesID{"squirtle", "pidgey", "rattata", "pidgeotto"}
	coverage.noteSuccess(Objective{Kind: KindTrain, Species: "pidgey", Slot: 1, Level: 18, Intent: "dex-evolution"}, afterCatch, afterEvolution)

	got := coverage.snapshot(afterEvolution, known)
	if got.UniqueMapsVisited != 2 || got.TrainersDefeated != 1 || got.NPCInteractions != 1 {
		t.Fatalf("interaction coverage = %+v", got)
	}
	if got.UniqueItemsAcquired != 2 || got.UniqueItemsUsed != 1 || got.TMsHMsAcquired != 1 || got.TMsHMsUsed != 1 {
		t.Fatalf("item coverage = %+v", got)
	}
	if got.DexOwned != 4 || got.DexSeen != 4 || got.UniqueSpeciesAcquired != 4 {
		t.Fatalf("dex coverage = %+v", got)
	}
	if got.Catches != 1 || got.Evolutions != 1 {
		t.Fatalf("collection coverage = %+v", got)
	}
	if got.OptionalMilestones < 4 {
		t.Fatalf("optional milestones = %d, want trainer/item/catch/evolution coverage", got.OptionalMilestones)
	}
}

func TestCoveragePersistsBesideAndInsideCheckpointKnowledge(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-001-frame-0000000100-test.state")
	if err := os.WriteFile(statePath, []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	known := NewKnowledge(nil)
	if err := writeMemoryFile(statePath, known, "", 0); err != nil {
		t.Fatal(err)
	}

	coverage := newCoverageTracker()
	coverage.ItemsAcquired["potion"] = true
	coverage.ItemsUsed["potion"] = true
	coverage.Trainers[objectiveStorageKey(Objective{Kind: KindTrainer, Location: "route-1", X: 3, Y: 4})] = true
	coverage.Catches = 2
	if err := writeCoverageFile(statePath, coverage); err != nil {
		t.Fatal(err)
	}
	if err := embedCoverageInKnowledgeFile(statePath, coverage); err != nil {
		t.Fatal(err)
	}

	loaded := loadCoverageFile(statePath, nil)
	if len(loaded.ItemsAcquired) != 1 || len(loaded.ItemsUsed) != 1 || len(loaded.Trainers) != 1 || loaded.Catches != 2 {
		t.Fatalf("sidecar round-trip = %+v", loaded)
	}

	if err := os.Remove(coveragePathForState(statePath)); err != nil {
		t.Fatal(err)
	}
	loaded = loadCoverageFile(statePath, nil)
	if len(loaded.ItemsAcquired) != 1 || len(loaded.ItemsUsed) != 1 || len(loaded.Trainers) != 1 || loaded.Catches != 2 {
		t.Fatalf("embedded farm-artifact round-trip = %+v", loaded)
	}
}

func TestCoverageMachineRecognitionIsSemantic(t *testing.T) {
	for _, id := range []ItemID{"tm01", "TM24", "hm03", "HM05"} {
		if !coverageMachineItem(id) {
			t.Fatalf("%q should be recognized as a machine", id)
		}
	}
	for _, id := range []ItemID{"potion", "masterball", "item_tm"} {
		if coverageMachineItem(id) {
			t.Fatalf("%q should not be recognized as a machine", id)
		}
	}
}
