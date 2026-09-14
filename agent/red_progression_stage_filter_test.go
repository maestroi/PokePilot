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

func hasObjective(objs []Objective, want Objective) bool {
	for _, o := range objs {
		if o.Kind == want.Kind && o.Place == want.Place && o.Progress == want.Progress {
			return true
		}
	}
	return false
}
