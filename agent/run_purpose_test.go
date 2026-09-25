package agent

import (
	"strings"
	"testing"
)

func TestDebugCoveragePurposeAnnotatesNovelSurfaces(t *testing.T) {
	obs := Observation{
		PartyCount:   1,
		Party:        []PartyMon{{Species: "squirtle", HP: 20, MaxHP: 20}},
		PokedexOwned: []SpeciesID{"squirtle"},
	}
	cases := []struct {
		name string
		obj  Objective
		tag  string
	}{
		{"map", Objective{Kind: KindGoTo, Note: "(unvisited adjacent map)"}, "new-map"},
		{"npc", Objective{Kind: KindTalk, X: 3, Y: 4}, "new-npc"},
		{"trainer", Objective{Kind: KindTrainer, X: 5, Y: 6}, "new-trainer"},
		{"pickup", Objective{Kind: KindPickup, Item: "potion"}, "new-pickup"},
		{"catch", Objective{Kind: KindCatch, Species: "rattata"}, "new-species"},
		{"evolution", Objective{Kind: KindTrain, Species: "caterpie", Intent: "dex-evolution"}, "new-evolution"},
		{"machine", Objective{Kind: KindUseItem, Item: "tm01"}, "machine-use"},
		{"shop", Objective{Kind: KindBuy, Item: "poke ball"}, "shop-flow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AnnotateRunPurpose(obs, []Objective{tc.obj}, RunPurposeDebugCoverage)
			if len(got) != 1 || !strings.Contains(got[0].Note, "debug-coverage") || !strings.Contains(got[0].Note, tc.tag) {
				t.Fatalf("annotation = %+v, want debug tag %q", got, tc.tag)
			}
		})
	}
}

func TestDebugCoveragePurposeSkipsOwnedCatchAndNormalRuns(t *testing.T) {
	obs := Observation{PokedexOwned: []SpeciesID{"rattata"}}
	catch := Objective{Kind: KindCatch, Species: "rattata"}
	if got := AnnotateRunPurpose(obs, []Objective{catch}, RunPurposeDebugCoverage); got[0].Note != "" {
		t.Fatalf("owned catch annotated as new coverage: %q", got[0].Note)
	}
	if got := AnnotateRunPurpose(obs, []Objective{{Kind: KindTalk}}, RunPurposeNormal); got[0].Note != "" {
		t.Fatalf("normal run received debug annotation: %q", got[0].Note)
	}
}

func TestRunPurposeSystemNote(t *testing.T) {
	if got := RunPurposeSystemNote(""); got != "" {
		t.Fatalf("legacy empty purpose changed prompt: %q", got)
	}
	got := RunPurposeSystemNote(RunPurposeDebugCoverage)
	for _, want := range []string{"DEBUG COVERAGE", "NEW reachable", "story progression as an unlock step"} {
		if !strings.Contains(got, want) {
			t.Fatalf("debug purpose note = %q, want %q", got, want)
		}
	}
}

func TestApplyRunPurposeDexGoalPrioritizesAcquisition(t *testing.T) {
	obs := Observation{
		PartyCount: 1,
		Party:      []PartyMon{{Species: "mewtwo", HP: 100, MaxHP: 100}},
		Bag:        []Item{{Name: "pokeball", Quantity: minimumCaptureStock}},
		Dex:        DexCatalog{Targets: []DexEntry{{Species: "rattata"}}},
	}
	offered := []Objective{
		{Kind: KindProgress, Progress: ProgressID("next_story_gate")},
		{Kind: KindTalk, X: 3, Y: 4},
		{Kind: KindCatch, Species: "rattata"},
	}

	got := ApplyRunPurpose(obs, offered, RunPurposeDebugCoverage, "Complete the obtainable Pokédex.")
	if len(got) != 1 || got[0].Kind != KindCatch || got[0].Species != "rattata" {
		t.Fatalf("Debug+Dex menu = %+v, want only executable Dex acquisition", got)
	}
}

func TestApplyRunPurposeDexGoalCreatesRemoteCaptureRestock(t *testing.T) {
	obs := Observation{
		PartyCount:   1,
		Party:        []PartyMon{{Species: "mewtwo", HP: 100, MaxHP: 100}},
		Money:        2000,
		RestockStock: []string{"pokeball", "potion"},
		Dex:          DexCatalog{Targets: []DexEntry{{Species: "rattata"}}},
	}
	offered := []Objective{
		{Kind: KindProgress, Progress: ProgressID("next_story_gate")},
		{Kind: KindTalk, X: 3, Y: 4},
	}

	got := ApplyRunPurpose(obs, offered, RunPurposeDebugCoverage, "dex")
	if len(got) != 1 || got[0].Kind != KindBuy || got[0].Item != "pokeball" ||
		got[0].Qty != targetCaptureStock || got[0].Intent != dexCaptureSupplyIntent {
		t.Fatalf("Debug+Dex zero-ball menu = %+v, want remote capture restock", got)
	}
}

func TestApplyRunPurposeDebugCoverageSweepsFrontierBeforeStory(t *testing.T) {
	obs := Observation{
		PartyCount: 1,
		Party:      []PartyMon{{Species: "mewtwo", HP: 100, MaxHP: 100}},
	}
	offered := []Objective{
		{Kind: KindProgress, Progress: ProgressID("next_story_gate")},
		{Kind: KindGoTo, Place: "cerulean city"},
		{Kind: KindTalk, X: 3, Y: 4},
		{Kind: KindPickup, Item: "potion", X: 5, Y: 6},
	}

	got := ApplyRunPurpose(obs, offered, RunPurposeDebugCoverage, "Beat the Elite Four and Champion.")
	if len(got) != 2 {
		t.Fatalf("Debug frontier menu = %+v, want talk + pickup only", got)
	}
	for _, objective := range got {
		if objective.Kind != KindTalk && objective.Kind != KindPickup {
			t.Fatalf("Debug frontier leaked non-coverage objective: %+v", objective)
		}
	}
}

func TestApplyRunPurposeAllowsStoryAfterCoverageFrontierIsEmpty(t *testing.T) {
	obs := Observation{
		PartyCount: 1,
		Party:      []PartyMon{{Species: "mewtwo", HP: 100, MaxHP: 100}},
	}
	offered := []Objective{
		{Kind: KindProgress, Progress: ProgressID("next_story_gate")},
		{Kind: KindGoTo, Place: "cerulean city"},
	}

	got := ApplyRunPurpose(obs, offered, RunPurposeDebugCoverage, "Beat the Elite Four and Champion.")
	if len(got) != len(offered) {
		t.Fatalf("empty coverage frontier menu = %+v, want story/travel menu preserved", got)
	}
}

func TestApplyRunPurposeKeepsRecoveryChoicesUnderSafetyPressure(t *testing.T) {
	obs := Observation{
		PartyCount: 1,
		Party:      []PartyMon{{Species: "mewtwo", HP: 10, MaxHP: 100}},
		Bag:        []Item{{Name: "pokeball", Quantity: minimumCaptureStock}},
		Dex:        DexCatalog{Targets: []DexEntry{{Species: "rattata"}}},
	}
	offered := []Objective{
		{Kind: KindHeal},
		{Kind: KindCatch, Species: "rattata"},
		{Kind: KindProgress, Progress: ProgressID("next_story_gate")},
	}

	got := ApplyRunPurpose(obs, offered, RunPurposeDebugCoverage, "dex")
	if len(got) != len(offered) {
		t.Fatalf("injured Debug+Dex menu = %+v, want safety menu preserved", got)
	}
}
