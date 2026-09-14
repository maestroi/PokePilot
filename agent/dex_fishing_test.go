package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestAppendDexCatchObjectivesOffersFishingWithOwnedRod(t *testing.T) {
	obs := Observation{
		Bag: []Item{
			{Name: "pokeball", Quantity: 5},
			{Name: "old rod", Quantity: 1},
		},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "magikarp",
			Sources: []DexSource{{Kind: AcquireFishing, Place: "route 6", Requirement: "old_rod"}},
		}}},
	}

	got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("fishing objectives = %+v, want one", got)
	}
	o := got[0]
	if o.Kind != KindCatch || o.Species != "magikarp" || o.Place != "route 6" || o.Item != "old rod" || o.Intent != dexFishingIntent || !o.Flee {
		t.Fatalf("fishing objective = %+v", o)
	}
	if !strings.Contains(strings.ToLower(o.Note), "dex fishing") || !strings.Contains(strings.ToLower(o.Note), "old rod") {
		t.Fatalf("fishing note = %q", o.Note)
	}
}

func TestAppendDexCatchObjectivesUsesVermilionForGlobalRodTables(t *testing.T) {
	vermilion, ok := skill.Place("vermilion city")
	if !ok {
		t.Fatal("vermilion city place missing")
	}
	obs := Observation{
		Map: vermilion.Map,
		Bag: []Item{
			{Name: "pokeball", Quantity: 5},
			{Name: "good rod", Quantity: 1},
		},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "goldeen",
			Sources: []DexSource{{Kind: AcquireFishing, Requirement: "good_rod"}},
		}}},
	}

	got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 || got[0].Place != dexGlobalFishingPlace || got[0].Item != "good rod" || got[0].Intent != dexFishingIntent {
		t.Fatalf("global fishing objective = %+v", got)
	}
}

func TestAppendDexCatchObjectivesRequiresOwnedRodAndSkipsSafariFishing(t *testing.T) {
	obs := Observation{
		Bag: []Item{{Name: "pokeball", Quantity: 5}},
		Dex: DexCatalog{Targets: []DexEntry{
			{Species: "magikarp", Sources: []DexSource{{Kind: AcquireFishing, Requirement: "old_rod"}}},
			{Species: "dratini", Sources: []DexSource{{Kind: AcquireFishing, Place: "safari zone center", Requirement: "super_rod+safari_zone"}}},
		}},
	}
	if got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("unexecutable fishing objectives = %+v, want none", got)
	}
}

func TestAppendDexCatchObjectivesPrefersGrassAtEqualDistance(t *testing.T) {
	route6, ok := skill.Place("route 6")
	if !ok {
		t.Fatal("route 6 place missing")
	}
	obs := Observation{
		Map: route6.Map,
		Bag: []Item{
			{Name: "pokeball", Quantity: 5},
			{Name: "super rod", Quantity: 1},
		},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "slowpoke",
			Sources: []DexSource{
				{Kind: AcquireFishing, Place: "route 6", Requirement: "super_rod"},
				{Kind: AcquireWildGrass, Place: "route 6"},
			},
		}}},
	}
	got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 || got[0].Intent == dexFishingIntent || got[0].Item != "" {
		t.Fatalf("equal-distance source choice = %+v, want ordinary grass catch", got)
	}
}
