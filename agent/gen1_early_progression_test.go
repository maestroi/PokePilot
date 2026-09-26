package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/gen1"
)

func TestGen1EarlyProgressionOrder(t *testing.T) {
	if got := gen1EarlyProgressionObjectives(Observation{}, false); len(got) != 0 {
		t.Fatalf("opening incomplete: got %v, want no shared progression", got)
	}

	afterOpening := gen1EarlyProgressionObjectives(Observation{}, true)
	if len(afterOpening) != 1 || afterOpening[0].Progress != gen1.ProgressPokedexAcquired {
		t.Fatalf("after opening = %v, want Pokedex", afterOpening)
	}

	afterDex := gen1EarlyProgressionObjectives(Observation{
		Story: ProgressState{{ID: gen1.ProgressPokedexAcquired, Complete: true}},
	}, true)
	if len(afterDex) != 1 || afterDex[0].Progress != gen1.ProgressBoulderBadge {
		t.Fatalf("after Pokedex = %v, want Boulder", afterDex)
	}

	done := gen1EarlyProgressionObjectives(Observation{
		Story: ProgressState{
			{ID: gen1.ProgressPokedexAcquired, Complete: true},
			{ID: gen1.ProgressBoulderBadge, Complete: true},
		},
	}, true)
	if len(done) != 0 {
		t.Fatalf("after Boulder = %v, want complete", done)
	}
}

func TestGen1EarlyExecutorsAreShared(t *testing.T) {
	for _, id := range []ProgressID{gen1.ProgressPokedexAcquired, gen1.ProgressBoulderBadge} {
		if run, ok := gen1EarlyProgressionExecutor(id); !ok || run == nil {
			t.Fatalf("shared executor %q missing", id)
		}
	}
	if _, ok := gen1EarlyProgressionExecutor("not_early_gen1"); ok {
		t.Fatal("unrelated progression registered as shared early Gen-I")
	}
}

func TestGen1EarlyRouteRequirementsUseSemanticFacts(t *testing.T) {
	blocked := gen1EarlyRouteRequirements(Observation{}, "test")
	if !routeRequirementsBlockMap(blocked, gen1Route2Map) || !routeRequirementsBlockMap(blocked, gen1Route3Map) {
		t.Fatalf("fresh early gates = %v, want Route 2 + Route 3 blocked", blocked)
	}

	blocked = gen1EarlyRouteRequirements(Observation{
		Story: ProgressState{{ID: gen1.ProgressPokedexAcquired, Complete: true}},
	}, "test")
	if routeRequirementsBlockMap(blocked, gen1Route2Map) || !routeRequirementsBlockMap(blocked, gen1Route3Map) {
		t.Fatalf("post-Pokedex gates = %v, want only Route 3 blocked", blocked)
	}

	blocked = gen1EarlyRouteRequirements(Observation{
		Story: ProgressState{
			{ID: gen1.ProgressPokedexAcquired, Complete: true},
			{ID: gen1.ProgressBoulderBadge, Complete: true},
		},
	}, "test")
	if routeRequirementsBlockMap(blocked, gen1Route2Map) || routeRequirementsBlockMap(blocked, gen1Route3Map) {
		t.Fatalf("post-Boulder gates = %v, want both open", blocked)
	}
}
