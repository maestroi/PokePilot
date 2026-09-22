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

func TestYellowMiddleProgressionUsesSharedOrder(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	base := []ProgressID{
		gen1.ProgressPokedexAcquired,
		gen1.ProgressBoulderBadge,
		gen1.ProgressMtMoonFossilAcquired,
		yellowprofile.ProgressYellowMtMoonExitResolved,
		gen1.ProgressSSTicketAcquired,
		gen1.ProgressHM01Acquired,
	}
	cases := []struct {
		name string
		done []ProgressID
		want ProgressID
	}{
		{"misty", nil, gen1.ProgressCascadeBadge},
		{"surge", []ProgressID{gen1.ProgressCascadeBadge}, gen1.ProgressThunderBadge},
		{"lavender", []ProgressID{gen1.ProgressCascadeBadge, gen1.ProgressThunderBadge}, gen1.ProgressPostSurgeLavenderReached},
		{"celadon", []ProgressID{gen1.ProgressCascadeBadge, gen1.ProgressThunderBadge, gen1.ProgressPostSurgeLavenderReached}, gen1.ProgressPostSurgeCeladonReady},
		{"erika", []ProgressID{gen1.ProgressCascadeBadge, gen1.ProgressThunderBadge, gen1.ProgressPostSurgeLavenderReached, gen1.ProgressPostSurgeCeladonReady}, gen1.ProgressRainbowBadge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			done := append(append([]ProgressID{}, base...), tc.done...)
			got := a.ProgressionObjectives(yellowProgressObservation(done...))
			if len(got) != 1 || got[0].Progress != tc.want {
				t.Fatalf("got=%v want progression %q", got, tc.want)
			}
		})
	}
}

func TestYellowProgressionResumesOpeningAfterStarterReceipt(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
		},
	}
	got := a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != yellowprofile.ProgressYellowLabRivalResolved {
		t.Fatalf("opening recovery progression=%v, want lab rival resolution", got)
	}
}

func TestYellowProgressionLeavesFreshOpeningToStarterProvider(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{GameID: yellowprofile.GameID}
	if got := a.ProgressionObjectives(obs); len(got) != 0 {
		t.Fatalf("fresh opening progression=%v, want starter provider ownership", got)
	}
}
