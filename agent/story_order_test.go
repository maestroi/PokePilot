package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestRedProgressionKeepsThunderBadgeAheadOfLaterStory(t *testing.T) {
	obs := Observation{
		Map: route12Map,
		Story: ProgressState{
			{ID: redProgressHM01Acquired, Complete: true},
		},
	}
	got := redProgressionObjectives(obs)
	if !hasProgressObjective(got, redProgressThunderBadge) {
		t.Fatalf("HM01 state away from Vermilion did not offer resumable Thunder Badge progression: %v", got)
	}
	for _, later := range []ProgressID{redProgressRainbowBadge, redProgressPokeFluteAcquired, redProgressFuchsiaProgressionComplete} {
		if hasProgressObjective(got, later) {
			t.Fatalf("later progression %q was offered before Thunder Badge", later)
		}
	}
}

func TestJourneyStoryOrderSurgeThenFluteThenFuchsia(t *testing.T) {
	preSurge := Observation{}
	for _, mapID := range []uint8{route9Map, route10Map, lavenderTownMap, celadonCityMap} {
		if !journeyProgressionBlocked(preSurge, mapID) {
			t.Fatalf("map %#02x should stay behind Thunder Badge", mapID)
		}
	}

	postSurge := Observation{Badges: []string{state.BadgeThunder.String()}}
	for _, mapID := range []uint8{route9Map, route10Map, lavenderTownMap, celadonCityMap} {
		if journeyProgressionBlocked(postSurge, mapID) {
			t.Fatalf("map %#02x should open after Thunder Badge", mapID)
		}
	}
	for _, mapID := range []uint8{route12Map, route13Map, route14Map, route15Map, fuchsiaCityMap} {
		if !journeyProgressionBlocked(postSurge, mapID) {
			t.Fatalf("later map %#02x should stay behind Poke Flute", mapID)
		}
	}

	postFlute := postSurge
	postFlute.Story = ProgressState{{ID: redProgressPokeFluteAcquired, Complete: true}}
	for _, mapID := range []uint8{route12Map, route13Map, route14Map, route15Map, fuchsiaCityMap} {
		if journeyProgressionBlocked(postFlute, mapID) {
			t.Fatalf("later map %#02x should open after Poke Flute", mapID)
		}
	}
}
