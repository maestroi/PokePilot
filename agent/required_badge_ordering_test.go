package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func TestCascadeProgressionRoutesBackToMistyAfterHM01(t *testing.T) {
	obs := Observation{
		Map: 0x05, // Vermilion City
		Story: ProgressState{
			{ID: redProgressHM01Acquired, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if !hasRequiredBadgeObjective(got, KindGoTo, "cerulean gym") {
		t.Fatalf("HM01 state without Cascade did not route back to Misty: %v", got)
	}
}

func TestCascadeProgressionChallengesMistyInsideGym(t *testing.T) {
	gym, ok := skill.Place("cerulean gym")
	if !ok {
		t.Fatal("cerulean gym place missing")
	}
	obs := Observation{
		Map: gym.Map,
		Story: ProgressState{
			{ID: redProgressHM01Acquired, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if !hasRequiredBadgeObjective(got, KindGym, "cerulean gym") {
		t.Fatalf("Cerulean Gym did not surface Misty as required progression: %v", got)
	}
}

func TestCascadeProgressionStopsAfterBadge(t *testing.T) {
	obs := Observation{
		Map:    0x05,
		Badges: []string{state.BadgeCascade.String()},
		Story: ProgressState{
			{ID: redProgressHM01Acquired, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if hasRequiredBadgeObjective(got, KindGym, "cerulean gym") || hasRequiredBadgeObjective(got, KindGoTo, "cerulean gym") {
		t.Fatalf("Cascade progression remained after Misty badge was present: %v", got)
	}
}

func TestRainbowBadgeBlocksSilphScopeUntilErika(t *testing.T) {
	obs := Observation{
		Map:    0xC7, // Rocket Hideout B1F
		Badges: []string{state.BadgeThunder.String()},
		Story: ProgressState{
			{ID: redProgressPostSurgeLavenderReached, Complete: true},
			{ID: redProgressPostSurgeCeladonReady, Complete: true},
			{ID: redProgressRainbowBadge, Complete: false},
		},
	}

	without := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if !offeredProgressID(without, redProgressRainbowBadge) {
		t.Fatalf("Rainbow progression missing before Erika: %v", without)
	}
	if offeredProgressID(without, redProgressSilphScopeAcquired) {
		t.Fatalf("Silph Scope leaked before Rainbow Badge: %v", without)
	}

	obs.Badges = append(obs.Badges, state.BadgeRainbow.String())
	obs.Story = append(obs.Story, ProgressFact{ID: redProgressRainbowBadge, Complete: true})
	with := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if !offeredProgressID(with, redProgressSilphScopeAcquired) {
		t.Fatalf("Silph Scope missing after Rainbow Badge: %v", with)
	}
}

func TestRainbowBadgeRepairsOldPostFuchsiaStateBeforeSaffron(t *testing.T) {
	obs := Observation{
		Map:    saffronCityMap,
		Badges: []string{state.BadgeThunder.String(), state.BadgeSoul.String()},
		Story: ProgressState{
			{ID: redProgressFuchsiaProgressionComplete, Complete: true},
			{ID: ProgressSaffronGateOpen, Complete: false},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if offeredProgressID(got, ProgressSaffronGateOpen) {
		t.Fatalf("Saffron progression leaked from a historical no-Rainbow Fuchsia state: %v", got)
	}
}

func hasRequiredBadgeObjective(objectives []Objective, kind Kind, place PlaceID) bool {
	for _, objective := range objectives {
		if objective.Kind == kind && objective.Place == place {
			return true
		}
	}
	return false
}
