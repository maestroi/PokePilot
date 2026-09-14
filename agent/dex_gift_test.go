package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestAppendDexGiftObjectivesOffersEeveeWithoutPokeBalls(t *testing.T) {
	dest, ok := skill.Place("celadon mansion eevee")
	if !ok {
		t.Fatal("celadon mansion eevee place missing")
	}
	obs := Observation{
		Map: dest.Map,
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "eevee",
			Sources: []DexSource{{Kind: AcquireGift, Place: "celadon mansion eevee"}},
		}}},
	}

	got := appendDexGiftObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("gift objectives = %+v, want one", got)
	}
	o := got[0]
	if o.Kind != KindCatch || o.Species != "eevee" || o.Place != "celadon mansion eevee" || o.Intent != dexGiftIntent || !o.Flee {
		t.Fatalf("Eevee gift objective = %+v", o)
	}
	if !strings.Contains(strings.ToLower(o.Note), "dex gift") || !strings.Contains(strings.ToLower(o.Note), "nickname") {
		t.Fatalf("Eevee gift note = %q", o.Note)
	}
}

func TestAppendDexGiftObjectivesSuppressesOwnedEevee(t *testing.T) {
	dest, ok := skill.Place("celadon mansion eevee")
	if !ok {
		t.Fatal("celadon mansion eevee place missing")
	}
	obs := Observation{
		Map:          dest.Map,
		PokedexOwned: []SpeciesID{"eevee"},
		Dex: DexCatalog{
			Owned: []DexEntry{{Species: "eevee"}},
			Targets: []DexEntry{{
				Species: "eevee",
				Sources: []DexSource{{Kind: AcquireGift, Place: "celadon mansion eevee"}},
			}},
		},
	}
	if got := appendDexGiftObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("owned Eevee gift objectives = %+v, want none", got)
	}
}

func TestAppendDexGiftObjectivesDoesNotInventUnimplementedGiftExecution(t *testing.T) {
	obs := Observation{
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "lapras",
			Sources: []DexSource{{Kind: AcquireGift, Place: "silph co"}},
		}}},
	}
	if got := appendDexGiftObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("unsupported gift objectives = %+v, want none", got)
	}
}

func TestRedScriptedSourcesLocatesEeveeGift(t *testing.T) {
	for _, scripted := range redScriptedSources() {
		if scripted.internal == 0x66 {
			if scripted.source.Kind != AcquireGift || scripted.source.Place != "celadon mansion eevee" {
				t.Fatalf("Eevee source = %+v", scripted.source)
			}
			return
		}
	}
	t.Fatal("Eevee scripted source missing")
}
