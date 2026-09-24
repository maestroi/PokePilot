package agent

import (
	"strings"
	"testing"
)

func TestDebugCoveragePurposeAnnotatesNovelSurfaces(t *testing.T) {
	obs := Observation{
		PartyCount: 1,
		Party:      []PartyMon{{Species: "squirtle", HP: 20, MaxHP: 20}},
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
	for _, want := range []string{"DEBUG COVERAGE", "NEW reachable", "Progress the story"} {
		if !strings.Contains(got, want) {
			t.Fatalf("debug purpose note = %q, want %q", got, want)
		}
	}
}
