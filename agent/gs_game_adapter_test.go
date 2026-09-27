package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

func TestGSObjectiveCatalogOffersThreeSemanticStarters(t *testing.T) {
	provider, ok := objectiveCatalogProviderFor(gsprofile.GoldGameID)
	if !ok {
		t.Fatal("Gold objective catalog provider is not registered")
	}
	catalog := provider.ObjectiveCatalog(Observation{GameID: gsprofile.GoldGameID, PartyCount: 0})
	want := []CatalogStarter{
		{Starter: skill.StarterChikorita, Species: "chikorita"},
		{Starter: skill.StarterCyndaquil, Species: "cyndaquil"},
		{Starter: skill.StarterTotodile, Species: "totodile"},
	}
	if len(catalog.Starters) != len(want) {
		t.Fatalf("starters = %+v, want %+v", catalog.Starters, want)
	}
	for i := range want {
		if catalog.Starters[i] != want[i] {
			t.Fatalf("starter %d = %+v, want %+v", i, catalog.Starters[i], want[i])
		}
	}
}

func TestGSObjectiveCatalogStopsOfferingStarterAfterPartyExists(t *testing.T) {
	provider, _ := objectiveCatalogProviderFor(gsprofile.SilverGameID)
	catalog := provider.ObjectiveCatalog(Observation{GameID: gsprofile.SilverGameID, PartyCount: 1})
	if len(catalog.Starters) != 0 {
		t.Fatalf("starters = %+v, want none", catalog.Starters)
	}
}

func TestGSStarterValidationBindsChoiceToSemanticSpecies(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	for _, tc := range []struct {
		starter skill.Starter
		species SpeciesID
	}{
		{skill.StarterChikorita, "chikorita"},
		{skill.StarterCyndaquil, "cyndaquil"},
		{skill.StarterTotodile, "totodile"},
	} {
		if err := adapter.Validate(Objective{Kind: KindStarter, Starter: tc.starter, Species: tc.species}, Observation{}); err != nil {
			t.Fatalf("%s: Validate: %v", tc.species, err)
		}
	}
	err := adapter.Validate(Objective{Kind: KindStarter, Starter: skill.StarterCyndaquil, Species: "totodile"}, Observation{})
	if err == nil {
		t.Fatal("mismatched starter species validated")
	}
}

func TestGSStarterSpecsMatchElmBallLayout(t *testing.T) {
	tests := []struct {
		starter skill.Starter
		species game.SpeciesID
		x       uint8
	}{
		{skill.StarterCyndaquil, "cyndaquil", 6},
		{skill.StarterTotodile, "totodile", 7},
		{skill.StarterChikorita, "chikorita", 8},
	}
	for _, tc := range tests {
		spec, ok := gsStarterSpecFor(tc.starter)
		if !ok {
			t.Fatalf("%s: no starter spec", tc.species)
		}
		if spec.Species != tc.species || spec.BallX != tc.x || spec.BallY != 3 ||
			spec.ApproachX != tc.x || spec.ApproachY != 4 {
			t.Fatalf("%s spec = %+v", tc.species, spec)
		}
	}
}

func TestGSUnimplementedObjectiveNormalizesAsTypedBlock(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	err := adapter.Validate(Objective{Kind: KindGoTo, Place: "violet-city"}, Observation{})
	if !errors.Is(err, errGSControllerUnavailable) {
		t.Fatalf("Validate error = %v, want controller unavailable", err)
	}
	failure := adapter.NormalizeFailure(game.FailurePhaseValidation, err, Observation{})
	if failure.Class != game.FailureClassBlocked || failure.Recoverable || failure.Cause != "gen2_controller_unavailable" {
		t.Fatalf("failure = %+v, want non-recoverable typed block", failure)
	}
}

func TestGSObjectiveAdapterFactoriesAreRegistered(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		if _, err := objectiveAdapterFactoryFor(id); err != nil {
			t.Fatalf("%s objective adapter: %v", id, err)
		}
	}
}
