package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func yellowProgressObservation(done ...ProgressID) Observation {
	story := ProgressState{{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true}}
	for _, id := range done {
		story = append(story, ProgressFact{ID: id, Complete: true})
	}
	return Observation{GameID: yellowprofile.GameID, PartyCount: 1, Story: story}
}

func TestYellowEarlyProgressionUsesSharedOrderWithMtMoonOverride(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	cases := []struct {
		name string
		done []ProgressID
		want ProgressID
	}{
		{"pokedex", nil, gen1.ProgressPokedexAcquired},
		{"brock", []ProgressID{gen1.ProgressPokedexAcquired}, gen1.ProgressBoulderBadge},
		{"fossil", []ProgressID{gen1.ProgressPokedexAcquired, gen1.ProgressBoulderBadge}, gen1.ProgressMtMoonFossilAcquired},
		{"yellow mt moon exit", []ProgressID{
			gen1.ProgressPokedexAcquired,
			gen1.ProgressBoulderBadge,
			gen1.ProgressMtMoonFossilAcquired,
		}, yellowprofile.ProgressYellowMtMoonExitResolved},
		{"bill", []ProgressID{
			gen1.ProgressPokedexAcquired,
			gen1.ProgressBoulderBadge,
			gen1.ProgressMtMoonFossilAcquired,
			yellowprofile.ProgressYellowMtMoonExitResolved,
		}, gen1.ProgressSSTicketAcquired},
		{"hm01", []ProgressID{
			gen1.ProgressPokedexAcquired,
			gen1.ProgressBoulderBadge,
			gen1.ProgressMtMoonFossilAcquired,
			yellowprofile.ProgressYellowMtMoonExitResolved,
			gen1.ProgressSSTicketAcquired,
		}, gen1.ProgressHM01Acquired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := a.ProgressionObjectives(yellowProgressObservation(tc.done...))
			if len(got) != 1 || got[0].Kind != KindProgress || got[0].Progress != tc.want {
				t.Fatalf("got=%v want one progression %q", got, tc.want)
			}
		})
	}
}

func TestYellowEarlyProgressionStopsAfterHM01(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := yellowProgressObservation(
		gen1.ProgressPokedexAcquired,
		gen1.ProgressBoulderBadge,
		gen1.ProgressMtMoonFossilAcquired,
		yellowprofile.ProgressYellowMtMoonExitResolved,
		gen1.ProgressSSTicketAcquired,
		gen1.ProgressHM01Acquired,
	)
	if got := a.ProgressionObjectives(obs); len(got) != 0 {
		t.Fatalf("post-HM01 early campaign offered %v", got)
	}
}

func TestYellowProgressionWaitsForOpeningResolution(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{GameID: yellowprofile.GameID, PartyCount: 1}
	if got := a.ProgressionObjectives(obs); len(got) != 0 {
		t.Fatalf("progression offered before lab rival resolution: %v", got)
	}
	catalog := a.ObjectiveCatalog(obs)
	if len(catalog.Starters) != 1 || catalog.Starters[0].Species != "pikachu" {
		t.Fatalf("opening recovery starter catalog=%v, want pikachu", catalog.Starters)
	}
}
