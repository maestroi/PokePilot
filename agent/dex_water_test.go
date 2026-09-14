package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestAppendDexCatchObjectivesOffersSurfWaterWithPreparableCapability(t *testing.T) {
	vermilion, ok := skill.Place("vermilion city")
	if !ok {
		t.Fatal("vermilion city place missing")
	}
	obs := Observation{
		Map: vermilion.Map,
		Bag: []Item{{Name: "pokeball", Quantity: 5}},
		FieldCapabilities: []FieldCapability{{
			Name:       "surf",
			BadgeOwned: true,
			HMOwned:    true,
			Preparable: true,
		}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "tentacool",
			Sources: []DexSource{{Kind: AcquireWildWater, Place: "vermilion city", Requirement: "surf"}},
		}}},
	}

	got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("Surf objectives = %+v, want one", got)
	}
	o := got[0]
	if o.Kind != KindCatch || o.Species != "tentacool" || o.Place != "vermilion city" || o.Intent != dexWaterIntent || !o.Flee {
		t.Fatalf("Surf objective = %+v", o)
	}
	if !strings.Contains(strings.ToLower(o.Note), "surf hunt") {
		t.Fatalf("Surf note = %q", o.Note)
	}
}

func TestAppendDexCatchObjectivesRequiresSurfAndSkipsSafariWater(t *testing.T) {
	obs := Observation{
		Bag: []Item{{Name: "pokeball", Quantity: 5}},
		Dex: DexCatalog{Targets: []DexEntry{
			{Species: "tentacool", Sources: []DexSource{{Kind: AcquireWildWater, Place: "vermilion city", Requirement: "surf"}}},
			{Species: "dratini", Sources: []DexSource{{Kind: AcquireWildWater, Place: "safari zone center", Requirement: "surf+safari_zone"}}},
		}},
	}
	if got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("water objectives without Surf = %+v, want none", got)
	}

	obs.FieldCapabilities = []FieldCapability{{Name: "surf", Usable: true}}
	got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 || got[0].Species != "tentacool" || got[0].Intent != dexWaterIntent {
		t.Fatalf("usable Surf objectives = %+v, want only ordinary water target", got)
	}
}

func TestAppendDexCatchObjectivesPrefersGrassOverSurfAtEqualDistance(t *testing.T) {
	vermilion, ok := skill.Place("vermilion city")
	if !ok {
		t.Fatal("vermilion city place missing")
	}
	obs := Observation{
		Map:               vermilion.Map,
		Bag:               []Item{{Name: "pokeball", Quantity: 5}},
		FieldCapabilities: []FieldCapability{{Name: "surf", Usable: true}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "slowpoke",
			Sources: []DexSource{
				{Kind: AcquireWildWater, Place: "vermilion city", Requirement: "surf"},
				{Kind: AcquireWildGrass, Place: "vermilion city"},
			},
		}}},
	}
	got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 || got[0].Intent != "" {
		t.Fatalf("equal-distance source choice = %+v, want ordinary grass catch", got)
	}
}
