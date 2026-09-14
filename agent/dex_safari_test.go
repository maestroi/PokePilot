package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestAppendDexCatchObjectivesOffersSafariWithoutPokeBalls(t *testing.T) {
	fuchsia, ok := skill.Place("fuchsia city")
	if !ok {
		t.Fatal("fuchsia city place missing")
	}
	obs := Observation{
		Map:   fuchsia.Map,
		Money: dexSafariFee,
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "tauros",
			Sources: []DexSource{{Kind: AcquireWildGrass, Place: "safari zone east", Requirement: "safari_zone"}},
		}}},
	}

	got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("Safari objectives = %+v, want one without ordinary Poke Balls", got)
	}
	o := got[0]
	if o.Kind != KindCatch || o.Species != "tauros" || o.Place != "safari zone east" || o.Intent != dexSafariIntent {
		t.Fatalf("Safari objective = %+v", o)
	}
	if !strings.Contains(strings.ToLower(o.Note), "safari hunt") {
		t.Fatalf("Safari note = %q", o.Note)
	}
}

func TestAppendDexCatchObjectivesRequiresSafariFeeOutsideZone(t *testing.T) {
	fuchsia, ok := skill.Place("fuchsia city")
	if !ok {
		t.Fatal("fuchsia city place missing")
	}
	obs := Observation{
		Map:   fuchsia.Map,
		Money: dexSafariFee - 1,
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "kangaskhan",
			Sources: []DexSource{{Kind: AcquireWildGrass, Place: "safari zone west", Requirement: "safari_zone"}},
		}}},
	}
	if got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("Safari objectives with insufficient entry money = %+v, want none", got)
	}

	obs.Money = dexSafariFee
	if got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil); len(got) != 1 || got[0].Intent != dexSafariIntent {
		t.Fatalf("Safari objective after funding fee = %+v", got)
	}
}

func TestAppendDexCatchObjectivesAllowsActiveSafariSessionWithoutMoney(t *testing.T) {
	obs := Observation{
		Map:   0xD9,
		Money: 0,
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "tauros",
			Sources: []DexSource{{Kind: AcquireWildGrass, Place: "safari zone east", Requirement: "safari_zone"}},
		}}},
	}
	got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 || got[0].Intent != dexSafariIntent {
		t.Fatalf("active-session Safari objective = %+v", got)
	}
}

func TestAppendDexCatchObjectivesDoesNotTreatSafariGrassAsOrdinaryCatch(t *testing.T) {
	obs := Observation{
		Map:   0xD9,
		Money: 0,
		Bag:   []Item{{Name: "pokeball", Quantity: 20}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "tauros",
			Sources: []DexSource{{Kind: AcquireWildGrass, Place: "safari zone east", Requirement: "safari_zone"}},
		}}},
	}
	got := appendDexCatchObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 || got[0].Intent != dexSafariIntent {
		t.Fatalf("Safari grass misclassified = %+v", got)
	}
}
