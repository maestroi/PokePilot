package agent_test

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/red/sym"
)

func loadDexROM(t *testing.T) []byte {
	t.Helper()
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	return romData
}

func TestBuildDexCatalogFromROM(t *testing.T) {
	romData := loadDexROM(t)
	cat, err := agent.BuildDexCatalog(romData, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	covered := map[agent.SpeciesID]string{}
	record := func(bucket string, entries []agent.DexEntry) {
		for _, e := range entries {
			if prev, ok := covered[e.Species]; ok {
				t.Errorf("%s listed in both %s and %s", e.Species, prev, bucket)
			}
			covered[e.Species] = bucket
		}
	}
	record("owned", cat.Owned)
	record("targets", cat.Targets)
	record("unavailable", cat.Unavailable)
	if len(covered) != sym.PokedexCount {
		t.Fatalf("catalog covers %d species, want %d", len(covered), sym.PokedexCount)
	}

	assertTarget := func(id agent.SpeciesID, kind string) {
		t.Helper()
		for _, e := range cat.Targets {
			if e.Species != id {
				continue
			}
			for _, src := range e.Sources {
				if src.Kind == kind {
					return
				}
			}
			t.Fatalf("%s is a target but has no %s source: %+v", id, kind, e.Sources)
		}
		t.Fatalf("%s is not a target: unavailable=%v", id, unavailableOf(cat, id))
	}

	assertTarget("pidgey", agent.AcquireWildGrass)
	assertTarget("tentacool", agent.AcquireWildWater)
	assertTarget("magikarp", agent.AcquireFishing)
	assertTarget("wartortle", agent.AcquireLevelEvo)
	assertTarget("vileplume", agent.AcquireItemEvo)
	assertTarget("mr.mime", agent.AcquireInGameTrade)

	if entry := unavailableOf(cat, "alakazam"); entry.Unavailable != agent.UnavailableTradeEvolution {
		t.Fatalf("alakazam unavailable = %q, want %q", entry.Unavailable, agent.UnavailableTradeEvolution)
	}
	if entry := unavailableOf(cat, "mew"); entry.Unavailable != agent.UnavailableEventOnly {
		t.Fatalf("mew unavailable = %q, want %q", entry.Unavailable, agent.UnavailableEventOnly)
	}
	if entry := unavailableOf(cat, "sandshrew"); entry.Unavailable != agent.UnavailableNoLocalSource {
		t.Fatalf("sandshrew unavailable = %q, want %q", entry.Unavailable, agent.UnavailableNoLocalSource)
	}
}

func TestBuildDexCatalogForfeitsStarterLineFromROM(t *testing.T) {
	romData := loadDexROM(t)
	cat, err := agent.BuildDexCatalog(romData, []agent.SpeciesID{"charmander"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if entry := unavailableOf(cat, "squirtle"); entry.Unavailable != agent.UnavailableForfeited+":starter" {
		t.Fatalf("squirtle = %+v, want forfeited starter after Charmander is owned", entry)
	}
	if !ownedOf(cat, "charmander") {
		t.Fatal("owned Charmander missing from Owned")
	}
}

func unavailableOf(cat agent.DexCatalog, id agent.SpeciesID) agent.DexEntry {
	for _, e := range cat.Unavailable {
		if e.Species == id {
			return e
		}
	}
	return agent.DexEntry{}
}

func ownedOf(cat agent.DexCatalog, id agent.SpeciesID) bool {
	for _, e := range cat.Owned {
		if e.Species == id {
			return true
		}
	}
	return false
}
