package agent

import (
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

const testPokedexOrderOffset = 0x41024

func TestProjectPokedexUsesOwnedNotSeen(t *testing.T) {
	romData := make([]byte, testPokedexOrderOffset+190)
	romData[testPokedexOrderOffset+0x99-1] = 1  // Bulbasaur
	romData[testPokedexOrderOffset+0xB0-1] = 4  // Charmander
	romData[testPokedexOrderOffset+0x54-1] = 25 // Pikachu

	owned, seen := ProjectPokedex(romData, state.PokedexState{
		Seen:  []uint8{1, 25},
		Owned: []uint8{4, 25},
	})
	if want := []SpeciesID{"charmander", "pikachu"}; !reflect.DeepEqual(owned, want) {
		t.Fatalf("owned = %v, want %v", owned, want)
	}
	if want := []SpeciesID{"bulbasaur", "pikachu"}; !reflect.DeepEqual(seen, want) {
		t.Fatalf("seen = %v, want %v", seen, want)
	}
}

func TestProjectPokedexSkipsUnmappedDexNumbers(t *testing.T) {
	romData := make([]byte, testPokedexOrderOffset+190)
	romData[testPokedexOrderOffset+0x24-1] = 16 // Pidgey
	owned, seen := ProjectPokedex(romData, state.PokedexState{
		Owned: []uint8{16, 151},
		Seen:  []uint8{16},
	})
	if want := []SpeciesID{"pidgey"}; !reflect.DeepEqual(owned, want) {
		t.Fatalf("owned = %v, want %v", owned, want)
	}
	if !reflect.DeepEqual(seen, wantOwned("pidgey")) {
		t.Fatalf("seen = %v, want [pidgey]", seen)
	}
}

func TestAssembleDexCatalogClassifiesSourcesAndCompletion(t *testing.T) {
	species := []DexEntry{
		{Species: "pidgey", Dex: 16},
		{Species: "pidgeotto", Dex: 17},
		{Species: "alakazam", Dex: 65},
		{Species: "sandshrew", Dex: 27},
		{Species: "mew", Dex: 151},
		{Species: "charmander", Dex: 4},
	}
	sources := map[SpeciesID][]DexSource{
		"pidgey":     {{Kind: AcquireWildGrass, Place: "route 1"}},
		"pidgeotto":  {{Kind: AcquireLevelEvo, From: "pidgey", Level: 18}},
		"alakazam":   {{Kind: AcquireTradeEvo, From: "kadabra"}},
		"charmander": {{Kind: AcquireStarter, ExclusiveGroup: "starter"}},
	}

	cat := assembleDexCatalog(species, sources, []SpeciesID{"charmander"}, nil, nil)
	if !hasDex(cat.Owned, "charmander") {
		t.Fatalf("owned = %v, want charmander", names(cat.Owned))
	}
	if !hasDex(cat.Targets, "pidgey") || !hasDex(cat.Targets, "pidgeotto") {
		t.Fatalf("targets = %v, want pidgey and its level evolution", names(cat.Targets))
	}
	if entry, ok := findDex(cat.Unavailable, "alakazam"); !ok || entry.Unavailable != UnavailableTradeEvolution {
		t.Fatalf("alakazam = %+v, want trade_evolution", entry)
	}
	if entry, ok := findDex(cat.Unavailable, "sandshrew"); !ok || entry.Unavailable != UnavailableNoLocalSource {
		t.Fatalf("sandshrew = %+v, want no_local_source", entry)
	}
	if entry, ok := findDex(cat.Unavailable, "mew"); !ok || entry.Unavailable != UnavailableEventOnly {
		t.Fatalf("mew = %+v, want event_only", entry)
	}
	if hasDex(cat.Targets, "charmander") {
		t.Fatal("owned Charmander must not remain a target")
	}
}

func TestAssembleDexCatalogForfeitsMutuallyExclusiveChoices(t *testing.T) {
	species := []DexEntry{
		{Species: "charmander", Dex: 4},
		{Species: "charmeleon", Dex: 5},
		{Species: "squirtle", Dex: 7},
		{Species: "wartortle", Dex: 8},
		{Species: "omanyte", Dex: 138},
		{Species: "kabuto", Dex: 140},
		{Species: "hitmonlee", Dex: 106},
		{Species: "hitmonchan", Dex: 107},
		{Species: "flareon", Dex: 136},
		{Species: "jolteon", Dex: 135},
		{Species: "vaporeon", Dex: 134},
	}
	sources := map[SpeciesID][]DexSource{
		"charmander": {{Kind: AcquireStarter, ExclusiveGroup: "starter"}},
		"squirtle":   {{Kind: AcquireStarter, ExclusiveGroup: "starter"}},
		"charmeleon": {{Kind: AcquireLevelEvo, From: "charmander", Level: 16}},
		"wartortle":  {{Kind: AcquireLevelEvo, From: "squirtle", Level: 16}},
		"omanyte":    {{Kind: AcquireFossil, ExclusiveGroup: "mt_moon_fossil"}},
		"kabuto":     {{Kind: AcquireFossil, ExclusiveGroup: "mt_moon_fossil"}},
		"hitmonlee":  {{Kind: AcquireGift, ExclusiveGroup: "fighting_dojo"}},
		"hitmonchan": {{Kind: AcquireGift, ExclusiveGroup: "fighting_dojo"}},
		"flareon":    {{Kind: AcquireItemEvo, From: "eevee", Item: "fire stone"}},
		"jolteon":    {{Kind: AcquireItemEvo, From: "eevee", Item: "thunder stone"}},
		"vaporeon":   {{Kind: AcquireItemEvo, From: "eevee", Item: "water stone"}},
	}

	cat := assembleDexCatalog(species, sources, []SpeciesID{"charmander", "omanyte", "hitmonlee", "flareon"}, nil, redExclusiveChoices())

	if entry, ok := findDex(cat.Unavailable, "squirtle"); !ok || entry.Unavailable != UnavailableForfeited+":starter" {
		t.Fatalf("squirtle = %+v, want forfeited starter", entry)
	}
	if entry, ok := findDex(cat.Unavailable, "wartortle"); !ok || entry.Unavailable != UnavailableForfeited+":starter" {
		t.Fatalf("wartortle = %+v, want forfeited with its starter line", entry)
	}
	if hasDex(cat.Targets, "kabuto") {
		t.Fatal("Dome fossil must be forfeited after Helix was taken")
	}
	if entry, ok := findDex(cat.Unavailable, "hitmonchan"); !ok || entry.Unavailable != UnavailableForfeited+":fighting_dojo" {
		t.Fatalf("hitmonchan = %+v, want forfeited fighting_dojo", entry)
	}
	if entry, ok := findDex(cat.Unavailable, "jolteon"); !ok || entry.Unavailable != UnavailableForfeited+":eevee_stone" {
		t.Fatalf("jolteon = %+v, want forfeited eevee_stone once Flareon exists and Eevee does not", entry)
	}
	if !hasDex(cat.Targets, "charmeleon") {
		t.Fatal("Charmeleon should stay obtainable from the owned Charmander")
	}
}

func TestAssembleDexCatalogIsDeterministic(t *testing.T) {
	species := []DexEntry{
		{Species: "pidgey", Dex: 16},
		{Species: "rattata", Dex: 19},
	}
	sources := map[SpeciesID][]DexSource{
		"rattata": {{Kind: AcquireWildGrass, Place: "route 2"}, {Kind: AcquireWildGrass, Place: "route 1"}},
		"pidgey":  {{Kind: AcquireWildGrass, Place: "route 1"}},
	}
	a := assembleDexCatalog(species, sources, nil, nil, nil)
	b := assembleDexCatalog(species, sources, nil, nil, nil)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("catalog is not deterministic:\n%+v\n%+v", a, b)
	}
	if a.Targets[0].Species != "pidgey" || a.Targets[1].Species != "rattata" {
		t.Fatalf("target order = %v, want dex order", names(a.Targets))
	}
	if a.Targets[1].Sources[0].Place != "route 1" {
		t.Fatalf("rattata sources = %+v, want place-sorted", a.Targets[1].Sources)
	}
}

func wantOwned(id SpeciesID) []SpeciesID { return []SpeciesID{id} }

func hasDex(entries []DexEntry, id SpeciesID) bool {
	_, ok := findDex(entries, id)
	return ok
}

func findDex(entries []DexEntry, id SpeciesID) (DexEntry, bool) {
	for _, e := range entries {
		if e.Species == id {
			return e, true
		}
	}
	return DexEntry{}, false
}

func names(entries []DexEntry) []SpeciesID {
	out := make([]SpeciesID, len(entries))
	for i, e := range entries {
		out[i] = e.Species
	}
	return out
}
