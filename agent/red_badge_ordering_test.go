package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestBadgeOrderingRoutesToMistyAfterSSTicket(t *testing.T) {
	obs := Observation{
		Map: 0x03,
		Story: ProgressState{
			{ID: redProgressSSTicketAcquired, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindGoTo || got[0].Place != "cerulean gym" {
		t.Fatalf("post-Bill progression did not require Cerulean Gym before later story beats: %v", got)
	}
}

func TestBadgeOrderingChallengesMistyInsideCeruleanGym(t *testing.T) {
	obs := Observation{
		Map: ceruleanGymMap,
		Story: ProgressState{
			{ID: redProgressSSTicketAcquired, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindGym || got[0].Place != "cerulean gym" {
		t.Fatalf("Cerulean Gym did not surface Misty as the required next story step: %v", got)
	}
}

func TestBadgeOrderingReleasesCutStoryAfterCascade(t *testing.T) {
	obs := Observation{
		Map:    0x03,
		Badges: []string{state.BadgeCascade.String()},
		Story: ProgressState{
			{ID: redProgressSSTicketAcquired, Complete: true},
		},
	}

	got := (&redObjectiveAdapter{}).ProgressionObjectives(obs)
	if !offeredProgressID(got, redProgressHM01Acquired) {
		t.Fatalf("HM01 progression missing after Cascade Badge: %v", got)
	}
}

func TestBadgeOrderingSuppressesPostErikaStoryUntilRainbow(t *testing.T) {
	obs := Observation{Badges: []string{state.BadgeThunder.String()}}
	objectives := []Objective{
		{Kind: KindProgress, Progress: redProgressPostSurgeLavenderReached},
		{Kind: KindProgress, Progress: redProgressPostSurgeCeladonReady},
		{Kind: KindProgress, Progress: redProgressRainbowBadge},
		{Kind: KindProgress, Progress: redProgressSilphScopeAcquired},
		{Kind: KindProgress, Progress: redProgressPokeFluteAcquired},
		{Kind: KindProgress, Progress: redProgressFuchsiaProgressionComplete},
		{Kind: KindProgress, Progress: ProgressSaffronGateOpen},
	}

	got := redBadgeOrderedProgression(obs, objectives)
	for _, forbidden := range []ProgressID{
		redProgressSilphScopeAcquired,
		redProgressPokeFluteAcquired,
		redProgressFuchsiaProgressionComplete,
		ProgressSaffronGateOpen,
	} {
		if offeredProgressID(got, forbidden) {
			t.Fatalf("progression %q leaked before Rainbow Badge: %v", forbidden, got)
		}
	}
	for _, required := range []ProgressID{
		redProgressPostSurgeLavenderReached,
		redProgressPostSurgeCeladonReady,
		redProgressRainbowBadge,
	} {
		if !offeredProgressID(got, required) {
			t.Fatalf("required pre-Erika progression %q was filtered out: %v", required, got)
		}
	}
}

func TestBadgeOrderingReleasesPostErikaStoryAfterRainbow(t *testing.T) {
	obs := Observation{Badges: []string{state.BadgeThunder.String(), state.BadgeRainbow.String()}}
	objectives := []Objective{{Kind: KindProgress, Progress: redProgressSilphScopeAcquired}}
	got := redBadgeOrderedProgression(obs, objectives)
	if !offeredProgressID(got, redProgressSilphScopeAcquired) {
		t.Fatalf("post-Erika progression remained filtered after Rainbow Badge: %v", got)
	}
}
