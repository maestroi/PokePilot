package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestRedCascadeBadgeObjectivesAfterHM01(t *testing.T) {
	obs := Observation{Story: ProgressState{{ID: redProgressHM01Acquired, Complete: true}}}
	got := redCascadeBadgeObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindGoTo || got[0].Place != "cerulean gym" {
		t.Fatalf("Cascade handoff = %#v, want one journey to cerulean gym", got)
	}

	obs.Badges = []string{state.BadgeCascade.String()}
	if got := redCascadeBadgeObjectives(obs); len(got) != 0 {
		t.Fatalf("Cascade handoff after badge = %#v, want none", got)
	}
}

func TestRedRequireRainbowForPostCeladon(t *testing.T) {
	objectives := []Objective{
		{Kind: KindProgress, Progress: redProgressSilphScopeAcquired},
		{Kind: KindProgress, Progress: redProgressPokeFluteAcquired},
		{Kind: KindProgress, Progress: redProgressRainbowBadge},
	}

	without := redRequireRainbowForPostCeladon(Observation{}, append([]Objective(nil), objectives...))
	if len(without) != 1 || without[0].Progress != redProgressRainbowBadge {
		t.Fatalf("without Rainbow = %#v, want only Rainbow progression", without)
	}

	with := redRequireRainbowForPostCeladon(
		Observation{Badges: []string{state.BadgeRainbow.String()}},
		append([]Objective(nil), objectives...),
	)
	if len(with) != len(objectives) {
		t.Fatalf("with Rainbow = %#v, want all %d objectives", with, len(objectives))
	}
}

func TestRedProgressionDoesNotOfferHideoutOrTowerBeforeRainbow(t *testing.T) {
	obs := Observation{
		Badges:     []string{state.BadgeThunder.String()},
		PartyCount: 1,
		Party:      []PartyMon{{Level: 30, HP: 80, MaxHP: 80}},
		Map:        0xC7,
	}
	for _, objective := range (&redObjectiveAdapter{}).ProgressionObjectives(obs) {
		if objective.Kind == KindProgress && objective.Progress == redProgressSilphScopeAcquired {
			t.Fatalf("Silph Scope offered before Rainbow: %#v", objective)
		}
	}

	obs.Badges = append(obs.Badges, state.BadgeRainbow.String())
	found := false
	for _, objective := range (&redObjectiveAdapter{}).ProgressionObjectives(obs) {
		if objective.Kind == KindProgress && objective.Progress == redProgressSilphScopeAcquired {
			found = true
		}
	}
	if !found {
		t.Fatal("Silph Scope not offered after Rainbow")
	}
}
