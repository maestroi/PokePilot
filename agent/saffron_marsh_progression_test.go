package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestMarshProgressionRoutesToSaffronAfterSilphRescue(t *testing.T) {
	obs := Observation{
		Map: saffronCityMap,
		Story: ProgressState{
			{ID: redProgressSilphRescueComplete, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if !hasObjective(got, KindGoTo, "saffron gym") {
		t.Fatalf("post-Silph progression did not route to Saffron Gym: %v", got)
	}
	if offeredProgressID(got, ProgressSecretKeyOwned) {
		t.Fatalf("Secret Key progression leaked before Marsh Badge: %v", got)
	}
}

func TestMarshProgressionChallengesSabrinaInsideGym(t *testing.T) {
	obs := Observation{
		Map: saffronGymMap,
		Story: ProgressState{
			{ID: redProgressSilphRescueComplete, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if !hasObjective(got, KindGym, "saffron gym") {
		t.Fatalf("Saffron Gym did not surface Sabrina as the next story step: %v", got)
	}
}

func TestMarshProgressionAdvancesToCinnabarAfterBadge(t *testing.T) {
	obs := Observation{
		Map:     saffronGymMap,
		Badges:  []string{state.BadgeMarsh.String()},
		Story: ProgressState{
			{ID: redProgressSilphRescueComplete, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if hasObjective(got, KindGym, "saffron gym") || hasObjective(got, KindGoTo, "saffron gym") {
		t.Fatalf("Marsh step remained after badge was earned: %v", got)
	}
	if !offeredProgressID(got, ProgressSecretKeyOwned) {
		t.Fatalf("Secret Key progression missing after Marsh Badge: %v", got)
	}
}

func TestMarshProgressionDoesNotAppearBeforeSilphRescue(t *testing.T) {
	obs := Observation{Map: saffronCityMap}
	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if hasObjective(got, KindGym, "saffron gym") || hasObjective(got, KindGoTo, "saffron gym") {
		t.Fatalf("Sabrina progression appeared before Silph rescue: %v", got)
	}
}

func hasObjective(objectives []Objective, kind ObjectiveKind, place PlaceID) bool {
	for _, objective := range objectives {
		if objective.Kind == kind && objective.Place == place {
			return true
		}
	}
	return false
}
