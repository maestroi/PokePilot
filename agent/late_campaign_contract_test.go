package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func lateCampaignStory(extra ...ProgressFact) ProgressState {
	story := ProgressState{
		{ID: redProgressPokedexAcquired, Complete: true},
		{ID: redProgressMtMoonFossilAcquired, Complete: true},
		{ID: redProgressSSTicketAcquired, Complete: true},
		{ID: redProgressHM01Acquired, Complete: true},
		{ID: redProgressBicycleAcquired, Complete: true},
		{ID: redProgressSilphScopeAcquired, Complete: true},
		{ID: redProgressPokeFluteAcquired, Complete: true},
		{ID: redProgressFuchsiaProgressionComplete, Complete: true},
	}
	return append(story, extra...)
}

func lateCampaignBadges(extra ...state.Badge) []string {
	badges := []state.Badge{
		state.BadgeBoulder,
		state.BadgeCascade,
		state.BadgeThunder,
		state.BadgeRainbow,
		state.BadgeSoul,
	}
	for _, badge := range extra {
		badges = append(badges, badge)
	}
	out := make([]string, 0, len(badges))
	for _, badge := range badges {
		out = append(out, badge.String())
	}
	return out
}

func hasObjectiveKindPlace(objs []Objective, kind Kind, place PlaceID) bool {
	for _, obj := range objs {
		if obj.Kind == kind && obj.Place == place {
			return true
		}
	}
	return false
}

// This is the ROM-free composition contract behind the full-run frontier. The
// individual skills have focused tests; this test proves their objective
// handoffs form one closed late-game chain instead of relying on the strategist
// to rediscover a missing gym/story step after badge five.
func TestRedLateCampaignHandoffsFiveBadgesThroughVictoryRoad(t *testing.T) {
	adapter := &redObjectiveAdapter{}
	tests := []struct {
		name         string
		obs          Observation
		wantProgress ProgressID
		wantKind     Kind
		wantPlace    PlaceID
		forbid       []ProgressID
	}{
		{
			name: "five badges open Saffron",
			obs: Observation{
				Badges: lateCampaignBadges(),
				Story:  lateCampaignStory(),
			},
			wantProgress: ProgressSaffronGateOpen,
			forbid:       []ProgressID{ProgressCardKeyOwned, redProgressSilphRescueComplete, ProgressSecretKeyOwned, redProgressVolcanoBadge, redProgressEarthBadge},
		},
		{
			name: "Saffron open gets Card Key",
			obs: Observation{
				Badges: lateCampaignBadges(),
				Story: lateCampaignStory(
					ProgressFact{ID: ProgressSaffronGateOpen, Complete: true},
				),
			},
			wantProgress: ProgressCardKeyOwned,
			forbid:       []ProgressID{redProgressSilphRescueComplete, ProgressSecretKeyOwned, redProgressVolcanoBadge, redProgressEarthBadge},
		},
		{
			name: "Card Key clears Silph",
			obs: Observation{
				Badges: lateCampaignBadges(),
				Story: lateCampaignStory(
					ProgressFact{ID: ProgressSaffronGateOpen, Complete: true},
					ProgressFact{ID: ProgressCardKeyOwned, Complete: true},
				),
			},
			wantProgress: redProgressSilphRescueComplete,
			forbid:       []ProgressID{ProgressSecretKeyOwned, redProgressVolcanoBadge, redProgressEarthBadge},
		},
		{
			name: "Silph rescue hands off to Sabrina",
			obs: Observation{
				Badges: lateCampaignBadges(),
				Story: lateCampaignStory(
					ProgressFact{ID: ProgressSaffronGateOpen, Complete: true},
					ProgressFact{ID: ProgressCardKeyOwned, Complete: true},
					ProgressFact{ID: redProgressSilphRescueComplete, Complete: true},
				),
			},
			wantKind:  KindGoTo,
			wantPlace: "saffron gym",
			forbid:    []ProgressID{ProgressSecretKeyOwned, redProgressVolcanoBadge, redProgressEarthBadge},
		},
		{
			name: "six badges acquire Secret Key",
			obs: Observation{
				Badges: lateCampaignBadges(state.BadgeMarsh),
				Story: lateCampaignStory(
					ProgressFact{ID: ProgressSaffronGateOpen, Complete: true},
					ProgressFact{ID: ProgressCardKeyOwned, Complete: true},
					ProgressFact{ID: redProgressSilphRescueComplete, Complete: true},
				),
			},
			wantProgress: ProgressSecretKeyOwned,
			forbid:       []ProgressID{redProgressVolcanoBadge, redProgressEarthBadge},
		},
		{
			name: "Secret Key earns seventh badge",
			obs: Observation{
				Badges: lateCampaignBadges(state.BadgeMarsh),
				Story: lateCampaignStory(
					ProgressFact{ID: ProgressSaffronGateOpen, Complete: true},
					ProgressFact{ID: ProgressCardKeyOwned, Complete: true},
					ProgressFact{ID: redProgressSilphRescueComplete, Complete: true},
					ProgressFact{ID: ProgressSecretKeyOwned, Complete: true},
				),
			},
			wantProgress: redProgressVolcanoBadge,
			forbid:       []ProgressID{redProgressEarthBadge},
		},
		{
			name: "seven badges earn Earth",
			obs: Observation{
				Badges: lateCampaignBadges(state.BadgeMarsh, state.BadgeVolcano),
				Story: lateCampaignStory(
					ProgressFact{ID: ProgressSaffronGateOpen, Complete: true},
					ProgressFact{ID: ProgressCardKeyOwned, Complete: true},
					ProgressFact{ID: redProgressSilphRescueComplete, Complete: true},
					ProgressFact{ID: ProgressSecretKeyOwned, Complete: true},
					ProgressFact{ID: redProgressVolcanoBadge, Complete: true},
				),
			},
			wantProgress: redProgressEarthBadge,
		},
		{
			name: "eight badges start League approach",
			obs: Observation{
				Badges: lateCampaignBadges(state.BadgeMarsh, state.BadgeVolcano, state.BadgeEarth),
				Story: lateCampaignStory(
					ProgressFact{ID: ProgressSaffronGateOpen, Complete: true},
					ProgressFact{ID: ProgressCardKeyOwned, Complete: true},
					ProgressFact{ID: redProgressSilphRescueComplete, Complete: true},
					ProgressFact{ID: ProgressSecretKeyOwned, Complete: true},
					ProgressFact{ID: redProgressVolcanoBadge, Complete: true},
					ProgressFact{ID: redProgressEarthBadge, Complete: true},
				),
			},
			wantProgress: ProgressRoute22RivalResolved,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := adapter.ProgressionObjectives(tc.obs)
			if tc.wantProgress != "" && !hasProgressObjective(got, tc.wantProgress) {
				t.Fatalf("next progression %q missing from %v", tc.wantProgress, got)
			}
			if tc.wantPlace != "" && !hasObjectiveKindPlace(got, tc.wantKind, tc.wantPlace) {
				t.Fatalf("next handoff %v %q missing from %v", tc.wantKind, tc.wantPlace, got)
			}
			for _, forbidden := range tc.forbid {
				if hasProgressObjective(got, forbidden) {
					t.Fatalf("later progression %q leaked before prerequisite; offered=%v", forbidden, got)
				}
			}
		})
	}
}
