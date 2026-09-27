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

func TestRocketTowerStagesAreOrderedAndFresh(t *testing.T) {
	want := []game.ProgressID{
		ProgressSilphScopeAcquired,
		ProgressPokeFluteAcquired,
	}
	got := RocketTowerStages()
	if len(got) != len(want) {
		t.Fatalf("RocketTowerStages()=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("RocketTowerStages()[%d]=%q, want %q", i, got[i], want[i])
		}
	}
	got[0] = "mutated"
	if RocketTowerStages()[0] != ProgressSilphScopeAcquired {
		t.Fatal("RocketTowerStages returned mutable global storage")
	}
}

func TestFuchsiaStagesAreOrderedAndFresh(t *testing.T) {
	want := []game.ProgressID{ProgressFuchsiaProgressionComplete}
	got := FuchsiaStages()
	if len(got) != len(want) {
		t.Fatalf("FuchsiaStages()=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FuchsiaStages()[%d]=%q, want %q", i, got[i], want[i])
		}
	}
	got[0] = "mutated"
	if FuchsiaStages()[0] != ProgressFuchsiaProgressionComplete {
		t.Fatal("FuchsiaStages returned mutable global storage")
	}
}

func TestCinnabarStagesAreOrderedAndFresh(t *testing.T) {
	want := []game.ProgressID{ProgressSecretKeyOwned, ProgressVolcanoBadge}
	got := CinnabarStages()
	if len(got) != len(want) {
		t.Fatalf("CinnabarStages()=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("CinnabarStages()[%d]=%q, want %q", i, got[i], want[i])
		}
	}
	got[0] = "mutated"
	if CinnabarStages()[0] != ProgressSecretKeyOwned {
		t.Fatal("CinnabarStages returned mutable global storage")
	}
}

func TestSaffronStagesAreOrderedAndFresh(t *testing.T) {
	want := []game.ProgressID{
		ProgressSaffronGateOpen,
		ProgressCardKeyOwned,
		ProgressSilphRescueComplete,
		ProgressMarshBadge,
	}
	got := SaffronStages()
	if len(got) != len(want) {
		t.Fatalf("SaffronStages()=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SaffronStages()[%d]=%q, want %q", i, got[i], want[i])
		}
	}
	got[0] = "mutated"
	if SaffronStages()[0] != ProgressSaffronGateOpen {
		t.Fatal("SaffronStages returned mutable global storage")
	}
}

func TestLeagueApproachStagesAreOrderedAndFresh(t *testing.T) {
	want := []game.ProgressID{
		ProgressEarthBadge,
		ProgressRoute22RivalResolved,
		ProgressRoute23BadgeChecks,
		ProgressVictoryRoadCleared,
		ProgressIndigoPlateauReady,
	}
	got := LeagueApproachStages()
	if len(got) != len(want) {
		t.Fatalf("LeagueApproachStages()=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LeagueApproachStages()[%d]=%q, want %q", i, got[i], want[i])
		}
	}
	got[0] = "mutated"
	if LeagueApproachStages()[0] != ProgressEarthBadge {
		t.Fatal("LeagueApproachStages returned mutable global storage")
	}
}
