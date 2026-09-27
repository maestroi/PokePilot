package gen1

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestMiddleCampaignStagesAreOrderedAndFresh(t *testing.T) {
	want := []game.ProgressID{
		ProgressThunderBadge,
		ProgressPostSurgeLavenderReached,
		ProgressPostSurgeCeladonReady,
		ProgressRainbowBadge,
	}
	got := MiddleCampaignStages()
	if len(got) != len(want) {
		t.Fatalf("MiddleCampaignStages()=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("MiddleCampaignStages()[%d]=%q, want %q", i, got[i], want[i])
		}
	}
	got[0] = "mutated"
	if MiddleCampaignStages()[0] != ProgressThunderBadge {
		t.Fatal("MiddleCampaignStages returned mutable global storage")
	}
}

func TestFirstIncomplete(t *testing.T) {
	state := game.ProgressState{
		{ID: ProgressThunderBadge, Complete: true},
		{ID: ProgressPostSurgeLavenderReached, Complete: true},
	}
	got, ok := FirstIncomplete(state, MiddleCampaignStages())
	if !ok || got != ProgressPostSurgeCeladonReady {
		t.Fatalf("FirstIncomplete=%q,%v, want %q,true", got, ok, ProgressPostSurgeCeladonReady)
	}
	state = append(state,
		game.ProgressFact{ID: ProgressPostSurgeCeladonReady, Complete: true},
		game.ProgressFact{ID: ProgressRainbowBadge, Complete: true},
	)
	if got, ok := FirstIncomplete(state, MiddleCampaignStages()); ok || got != "" {
		t.Fatalf("complete campaign FirstIncomplete=%q,%v, want empty,false", got, ok)
	}
}
