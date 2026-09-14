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

func TestAppendDexGiftObjectivesOffersLaprasWithCardKey(t *testing.T) {
	dest, ok := skill.Place("silph co lapras")
	if !ok {
		t.Fatal("silph co lapras place missing")
	}
	obs := Observation{
		Map: dest.Map,
		Bag: []Item{{Name: "card key", Quantity: 1}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "lapras",
			Sources: []DexSource{{Kind: AcquireGift, Place: "silph co lapras", Requirement: "card_key"}},
		}}},
	}
	got := appendDexGiftObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 || got[0].Species != "lapras" || got[0].Place != "silph co lapras" || got[0].Intent != dexGiftIntent {
		t.Fatalf("Lapras gift objectives = %+v, want executable Lapras", got)
	}
}

func TestAppendDexGiftObjectivesRequiresCardKeyForLapras(t *testing.T) {
	dest, ok := skill.Place("silph co lapras")
	if !ok {
		t.Fatal("silph co lapras place missing")
	}
	obs := Observation{
		Map: dest.Map,
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "lapras",
			Sources: []DexSource{{Kind: AcquireGift, Place: "silph co lapras", Requirement: "card_key"}},
		}}},
	}
	if got := appendDexGiftObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("Lapras without Card Key objectives = %+v, want none", got)
	}
}

func TestAppendDexGiftObjectivesOffersOnlyOneFightingDojoChoice(t *testing.T) {
	dest, ok := skill.Place("fighting dojo hitmonlee")
	if !ok {
		t.Fatal("fighting dojo hitmonlee place missing")
	}
	obs := Observation{
		Map: dest.Map,
		Dex: DexCatalog{Targets: []DexEntry{
			{
				Species: "hitmonlee",
				Sources: []DexSource{{Kind: AcquireGift, Place: "fighting dojo hitmonlee", ExclusiveGroup: "fighting_dojo"}},
			},
			{
				Species: "hitmonchan",
				Sources: []DexSource{{Kind: AcquireGift, Place: "fighting dojo hitmonchan", ExclusiveGroup: "fighting_dojo"}},
			},
		}},
	}
	got := appendDexGiftObjectives(obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("Dojo gift objectives = %+v, want exactly one exclusive branch", got)
	}
	if got[0].Species != "hitmonlee" || got[0].Place != "fighting dojo hitmonlee" || got[0].Intent != dexGiftIntent {
		t.Fatalf("Dojo gift objective = %+v, want first deterministic branch", got[0])
	}
}

func TestAppendDexGiftObjectivesSuppressesConsumedFightingDojoChoice(t *testing.T) {
	dest, ok := skill.Place("fighting dojo hitmonchan")
	if !ok {
		t.Fatal("fighting dojo hitmonchan place missing")
	}
	obs := Observation{
		Map:          dest.Map,
		PokedexOwned: []SpeciesID{"hitmonlee"},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "hitmonchan",
			Sources: []DexSource{{Kind: AcquireGift, Place: "fighting dojo hitmonchan", ExclusiveGroup: "fighting_dojo"}},
		}}},
	}
	if got := appendDexGiftObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("consumed Dojo choice objectives = %+v, want none", got)
	}
}

func TestAppendDexGiftObjectivesDoesNotInventUnimplementedGiftExecution(t *testing.T) {
	obs := Observation{
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "porygon",
			Sources: []DexSource{{Kind: AcquireGift, Requirement: "game_corner"}},
		}}},
	}
	if got := appendDexGiftObjectives(obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("unsupported gift objectives = %+v, want none", got)
	}
}

func TestRedScriptedSourcesLocatesExecutableGifts(t *testing.T) {
	want := map[uint8]DexSource{
		0x66: {Kind: AcquireGift, Place: "celadon mansion eevee"},
		0x13: {Kind: AcquireGift, Place: "silph co lapras", Requirement: "card_key"},
		0x2B: {Kind: AcquireGift, Place: "fighting dojo hitmonlee", ExclusiveGroup: "fighting_dojo"},
		0x2C: {Kind: AcquireGift, Place: "fighting dojo hitmonchan", ExclusiveGroup: "fighting_dojo"},
	}
	for _, scripted := range redScriptedSources() {
		expected, ok := want[scripted.internal]
		if !ok {
			continue
		}
		if scripted.source.Kind != expected.Kind || scripted.source.Place != expected.Place || scripted.source.Requirement != expected.Requirement || scripted.source.ExclusiveGroup != expected.ExclusiveGroup {
			t.Fatalf("scripted source %#02x = %+v, want %+v", scripted.internal, scripted.source, expected)
		}
		delete(want, scripted.internal)
	}
	if len(want) != 0 {
		t.Fatalf("missing scripted gift sources: %+v", want)
	}
}
