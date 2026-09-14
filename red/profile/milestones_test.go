package profile

import (
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestMajorMilestoneLabelsStoryOrder(t *testing.T) {
	got := MajorMilestoneLabels(state.StoryFacts{
		MtMoonFossilAcquired:   true,
		PokedexAcquired:        true,
		LeagueChampionDefeated: true,
		MainStoryComplete:      true,
	})
	want := []string{"Pokédex", "Mt. Moon fossil", "Champion", "Hall of Fame"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
}

func TestMajorMilestoneLabelsEmpty(t *testing.T) {
	if got := MajorMilestoneLabels(state.StoryFacts{}); len(got) != 0 {
		t.Fatalf("empty facts = %v, want none", got)
	}
}
