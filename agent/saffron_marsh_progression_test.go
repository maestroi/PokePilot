package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestMarshProgressionRoutesToSaffronAfterSilphRescue(t *testing.T) {
	obs := Observation{
		Map:    saffronCityMap,
		Badges: []string{state.BadgeRainbow.String()},
		Story: ProgressState{
			{ID: redProgressRainbowBadge, Complete: true},
			{ID: redProgressSilphRescueComplete, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if !hasMarshStepObjective(got, KindGoTo, "saffron gym") {
		t.Fatalf("post-Silph progression did not route to Saffron Gym: %v", got)
	}
	if offeredProgressID(got, ProgressSecretKeyOwned) {
		t.Fatalf("Secret Key progression leaked before Marsh Badge: %v", got)
	}
}

func TestMarshProgressionChallengesSabrinaInsideGym(t *testing.T) {
	obs := Observation{
		Map:    saffronGymMap,
		Badges: []string{state.BadgeRainbow.String()},
		Story: ProgressState{
			{ID: redProgressRainbowBadge, Complete: true},
			{ID: redProgressSilphRescueComplete, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if !hasMarshStepObjective(got, KindGym, "saffron gym") {
		t.Fatalf("Saffron Gym did not surface Sabrina as the next story step: %v", got)
	}
}

func TestMarshProgressionAdvancesToCinnabarAfterBadge(t *testing.T) {
	obs := Observation{
		Map: saffronGymMap,
		Badges: []string{
			state.BadgeRainbow.String(),
			state.BadgeMarsh.String(),
		},
		Story: ProgressState{
			{ID: redProgressRainbowBadge, Complete: true},
			{ID: redProgressSilphRescueComplete, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if hasMarshStepObjective(got, KindGym, "saffron gym") || hasMarshStepObjective(got, KindGoTo, "saffron gym") {
		t.Fatalf("Marsh step remained after badge was earned: %v", got)
	}
	if !offeredProgressID(got, ProgressSecretKeyOwned) {
		t.Fatalf("Secret Key progression missing after Marsh Badge: %v", got)
	}
}

func TestMarshProgressionDoesNotAppearBeforeSilphRescue(t *testing.T) {
	obs := Observation{
		Map:    saffronCityMap,
		Badges: []string{state.BadgeRainbow.String()},
		Story: ProgressState{
			{ID: redProgressRainbowBadge, Complete: true},
		},
	}
	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if hasMarshStepObjective(got, KindGym, "saffron gym") || hasMarshStepObjective(got, KindGoTo, "saffron gym") {
		t.Fatalf("Sabrina progression appeared before Silph rescue: %v", got)
	}
}

func hasMarshStepObjective(objectives []Objective, kind Kind, place PlaceID) bool {
	for _, objective := range objectives {
		if objective.Kind == kind && objective.Place == place {
			return true
		}
	}
	return false
}
