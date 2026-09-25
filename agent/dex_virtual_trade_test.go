package agent

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

func TestVirtualTradeOffersTradeEvolutionWhenBaseIsInParty(t *testing.T) {
	obs := Observation{
		Services:     &RuntimeServices{VirtualTrader: true},
		Party:        []PartyMon{{Species: "kadabra", Level: 30}},
		PokedexOwned: []SpeciesID{"charmander", "kadabra"},
		Dex: DexCatalog{
			Owned: []DexEntry{{Species: "charmander", Owned: true}},
			Unavailable: []DexEntry{{
				Species: "alakazam", Unavailable: UnavailableTradeEvolution,
				Sources: []DexSource{{Kind: AcquireTradeEvo, From: "kadabra"}},
			}},
		},
	}
	got := appendDexVirtualTradeObjectives(obs, nil, nil)
	if len(got) != 1 {
		t.Fatalf("objectives = %+v, want one tradeback", got)
	}
	if got[0].Kind != KindCatch || got[0].Species != "alakazam" || got[0].Slot != 0 || got[0].Intent != dexVirtualTradebackIntent {
		t.Fatalf("objective = %+v, want Kadabra tradeback for Alakazam", got[0])
	}
}

func TestVirtualTradeOffersVersionAssistedSpeciesWithReplaceableDonor(t *testing.T) {
	obs := Observation{
		Services: &RuntimeServices{VirtualTrader: true},
		Party: []PartyMon{
			{Species: "charmander", Level: 28},
			{Species: "pidgey", Level: 12},
		},
		Dex: DexCatalog{
			Owned: []DexEntry{
				{Species: "charmander", Owned: true, Sources: []DexSource{{Kind: AcquireStarter}}},
				{Species: "pidgey", Owned: true, Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 1"}}},
			},
			Unavailable: []DexEntry{{Species: "pinsir", Unavailable: UnavailableNoLocalSource}},
		},
	}
	got := appendDexVirtualTradeObjectives(obs, nil, nil)
	if len(got) != 1 {
		t.Fatalf("objectives = %+v, want one version-assisted trade", got)
	}
	if got[0].Species != "pinsir" || got[0].Slot != 1 || got[0].Intent != dexVirtualVersionIntent {
		t.Fatalf("objective = %+v, want Pidgey donor for Pinsir", got[0])
	}
}

func TestVirtualTradePokedexPolicyCoversForfeitedChoice(t *testing.T) {
	obs := Observation{
		Services: &RuntimeServices{VirtualTrader: true},
		Party: []PartyMon{
			{Species: "charmander", Level: 25},
			{Species: "rattata", Level: 9},
		},
		Dex: DexCatalog{
			Owned:       []DexEntry{{Species: "rattata", Owned: true, Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 1"}}}},
			Unavailable: []DexEntry{{Species: "squirtle", Unavailable: UnavailableForfeited + ":starter"}},
		},
	}
	got := appendDexVirtualTradeObjectives(obs, nil, nil)
	if len(got) != 1 || got[0].Intent != dexVirtualPokedexIntent || got[0].Species != "squirtle" {
		t.Fatalf("objectives = %+v, want pokedex virtual trade for forfeited starter", got)
	}
}

func TestVirtualTradeSuppressedAfterMachineUnusable(t *testing.T) {
	obs := Observation{
		Services: &RuntimeServices{VirtualTrader: true},
		Party: []PartyMon{
			{Species: "charmander", Level: 28},
			{Species: "pidgey", Level: 12},
		},
		Dex: DexCatalog{
			Owned: []DexEntry{
				{Species: "pidgey", Owned: true, Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 1"}}},
			},
			Unavailable: []DexEntry{{Species: "vulpix", Unavailable: UnavailableNoLocalSource}},
		},
	}
	known := NewKnowledge(nil)
	known.noteMachineUnusable(Objective{
		Kind: KindCatch, Species: "vulpix", Intent: dexVirtualVersionIntent, Slot: 1,
	}, gameruntime.ErrMachineUnusable)
	if got := appendDexVirtualTradeObjectives(obs, known, nil); len(got) != 0 {
		t.Fatalf("objectives = %+v, want no virtual trade after a poisoned link", got)
	}
}

func TestVirtualTradeAllowedAgainOnNewBuild(t *testing.T) {
	obs := Observation{
		Services: &RuntimeServices{VirtualTrader: true},
		Party: []PartyMon{
			{Species: "charmander", Level: 28},
			{Species: "pidgey", Level: 12},
		},
		Dex: DexCatalog{
			Owned: []DexEntry{
				{Species: "pidgey", Owned: true, Sources: []DexSource{{Kind: AcquireWildGrass, Place: "route 1"}}},
			},
			Unavailable: []DexEntry{{Species: "vulpix", Unavailable: UnavailableNoLocalSource}},
		},
	}
	known := NewKnowledge(nil)
	known.Build = "build-a"
	known.noteMachineUnusable(Objective{
		Kind: KindCatch, Species: "vulpix", Intent: dexVirtualVersionIntent, Slot: 1,
	}, gameruntime.ErrMachineUnusable)
	known.Build = "build-b"
	got := appendDexVirtualTradeObjectives(obs, known, nil)
	if len(got) != 1 || got[0].Species != "vulpix" {
		t.Fatalf("objectives = %+v, want the trade offered again on a new build", got)
	}
}

func TestVirtualTradeNeverOffersEventOnlySpecies(t *testing.T) {
	obs := Observation{
		Services: &RuntimeServices{VirtualTrader: true},
		Party:    []PartyMon{{Species: "charmander"}, {Species: "pidgey"}},
		Dex: DexCatalog{
			Owned:       []DexEntry{{Species: "pidgey", Owned: true, Sources: []DexSource{{Kind: AcquireWildGrass}}}},
			Unavailable: []DexEntry{{Species: "mew", Unavailable: UnavailableEventOnly}},
		},
	}
	if got := appendDexVirtualTradeObjectives(obs, nil, nil); len(got) != 0 {
		t.Fatalf("event-only objectives = %+v, want none", got)
	}
}

func TestVirtualTradeRequiresAdvertisedServiceAndSafeDonor(t *testing.T) {
	base := Observation{
		Party: []PartyMon{{Species: "charmander"}, {Species: "pidgey"}},
		Dex: DexCatalog{
			Owned:       []DexEntry{{Species: "pidgey", Owned: true, Sources: []DexSource{{Kind: AcquireWildGrass}}}},
			Unavailable: []DexEntry{{Species: "pinsir", Unavailable: UnavailableNoLocalSource}},
		},
	}
	if got := appendDexVirtualTradeObjectives(base, nil, nil); len(got) != 0 {
		t.Fatalf("without service = %+v, want none", got)
	}

	base.Services = &RuntimeServices{VirtualTrader: true}
	base.FieldCapabilities = []FieldCapability{{PartySlot: 1, Learned: true}}
	if got := appendDexVirtualTradeObjectives(base, nil, nil); len(got) != 0 {
		t.Fatalf("only replaceable donor carries field move: %+v, want none", got)
	}
}
