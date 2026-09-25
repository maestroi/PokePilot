package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestRedProgressionStageFilterBlocksErikaBypassUntilCeladonReady(t *testing.T) {
	obs := Observation{
		Badges: []string{state.BadgeBoulder.String(), state.BadgeCascade.String(), state.BadgeThunder.String()},
		Story: ProgressState{
			{ID: redProgressPostSurgeLavenderReached, Complete: true},
		},
	}
	in := []Objective{
		{Kind: KindProgress, Progress: redProgressPostSurgeCeladonReady},
		{Kind: KindGym, Place: "celadon gym"},
		{Kind: KindGoTo, Place: "celadon gym"},
		{Kind: KindGoTo, Place: "celadon pokemon center"},
		{Kind: KindGym, Place: "vermilion gym"},
	}

	got := filterRedProgressionStageObjectives(obs, in)
	for _, o := range got {
		if o.Place == "celadon gym" && (o.Kind == KindGym || o.Kind == KindGoTo) {
			t.Fatalf("Celadon gym bypass survived before recovery stage: %v", got)
		}
	}
	if !hasProgressObjective(got, redProgressPostSurgeCeladonReady) {
		t.Fatalf("required Celadon recovery stage was removed: %v", got)
	}
	if !hasObjective(got, Objective{Kind: KindGoTo, Place: "celadon pokemon center"}) {
		t.Fatalf("safe Center journey was removed: %v", got)
	}
	if !hasObjective(got, Objective{Kind: KindGym, Place: "vermilion gym"}) {
		t.Fatalf("unrelated gym objective was removed: %v", got)
	}
}

func TestRedProgressionStageFilterAllowsErikaAfterCeladonReady(t *testing.T) {
	obs := Observation{
		Badges: []string{state.BadgeBoulder.String(), state.BadgeCascade.String(), state.BadgeThunder.String()},
		Story: ProgressState{
			{ID: redProgressPostSurgeLavenderReached, Complete: true},
			{ID: redProgressPostSurgeCeladonReady, Complete: true},
		},
	}
	gym := Objective{Kind: KindGym, Place: "celadon gym"}
	journey := Objective{Kind: KindGoTo, Place: "celadon gym"}
	got := filterRedProgressionStageObjectives(obs, []Objective{gym, journey})
	if !hasObjective(got, gym) || !hasObjective(got, journey) {
		t.Fatalf("Celadon-ready filter removed legal Erika objectives: %v", got)
	}
}

func TestRedProgressionStageFilterIsInactiveOutsidePostSurgeWindow(t *testing.T) {
	gym := Objective{Kind: KindGym, Place: "celadon gym"}
	for _, obs := range []Observation{
		{},
		{Badges: []string{state.BadgeThunder.String(), state.BadgeRainbow.String()}},
	} {
		got := filterRedProgressionStageObjectives(obs, []Objective{gym})
		if !hasObjective(got, gym) {
			t.Fatalf("filter removed Celadon gym outside active post-Surge staging: obs=%+v got=%v", obs, got)
		}
	}
}

func TestRedProgressionStageFilterBlocksCinnabarGymWithOnlySilphCardKey(t *testing.T) {
	obs := Observation{
		Map: cinnabarIslandMap,
		Badges: []string{
			state.BadgeSoul.String(),
			state.BadgeMarsh.String(),
		},
		Story: ProgressState{
			{ID: ProgressCardKeyOwned, Complete: true},
			{ID: redProgressSilphRescueComplete, Complete: true},
			{ID: ProgressSecretKeyOwned, Complete: false},
		},
	}
	in := []Objective{
		{Kind: KindProgress, Progress: ProgressSecretKeyOwned},
		{Kind: KindGym, Place: "cinnabar gym"},
		{Kind: KindGoTo, Place: "cinnabar gym"},
		{Kind: KindGoTo, Place: "pokemon mansion"},
	}

	got := filterRedProgressionStageObjectives(obs, in)
	if hasObjective(got, Objective{Kind: KindGym, Place: "cinnabar gym"}) {
		t.Fatalf("Silph Card Key incorrectly left Cinnabar gym challenge selectable: %v", got)
	}
	if hasObjective(got, Objective{Kind: KindGoTo, Place: "cinnabar gym"}) {
		t.Fatalf("Silph Card Key incorrectly left Cinnabar gym journey selectable: %v", got)
	}
	if !hasProgressObjective(got, ProgressSecretKeyOwned) {
		t.Fatalf("Pokemon Mansion Secret Key progression was removed: %v", got)
	}
	if !hasObjective(got, Objective{Kind: KindGoTo, Place: "pokemon mansion"}) {
		t.Fatalf("Pokemon Mansion journey was removed: %v", got)
	}
}

func TestRedProgressionStageFilterAllowsCinnabarGymAfterMansionSecretKey(t *testing.T) {
	obs := Observation{
		Map: cinnabarIslandMap,
		Story: ProgressState{
			{ID: ProgressCardKeyOwned, Complete: true},
			{ID: ProgressSecretKeyOwned, Complete: true},
		},
	}
	gym := Objective{Kind: KindGym, Place: "cinnabar gym"}
	journey := Objective{Kind: KindGoTo, Place: "cinnabar gym"}

	got := filterRedProgressionStageObjectives(obs, []Objective{gym, journey})
	if !hasObjective(got, gym) || !hasObjective(got, journey) {
		t.Fatalf("Mansion Secret Key did not unlock Cinnabar gym candidates: %v", got)
	}
}

func TestRedProgressionStageFilterKeepsInsideCinnabarCheckpointRecoverable(t *testing.T) {
	obs := Observation{Map: cinnabarGymMap}
	gym := Objective{Kind: KindGym, Place: "cinnabar gym"}

	got := filterRedProgressionStageObjectives(obs, []Objective{gym})
	if !hasObjective(got, gym) {
		t.Fatalf("already-inside Cinnabar checkpoint lost local gym recovery: %v", got)
	}
}

func hasObjective(objs []Objective, want Objective) bool {
	for _, o := range objs {
		if o.Kind == want.Kind && o.Place == want.Place && o.Progress == want.Progress {
			return true
		}
	}
	return false
}
