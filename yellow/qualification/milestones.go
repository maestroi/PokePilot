// Package qualification defines the semantic checkpoints used to qualify a
// Pokémon Yellow campaign. It contains no native event numbers or RAM
// addresses: those remain owned by yellow/profile.
package qualification

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

type Milestone struct {
	ID          string
	Description string
	Reached     func(game.ProfileObservation) bool
}

func progress(id game.ProgressID) func(game.ProfileObservation) bool {
	return func(obs game.ProfileObservation) bool { return obs.Story.Has(id) }
}

func badgeCount(n int) func(game.ProfileObservation) bool {
	return func(obs game.ProfileObservation) bool { return len(obs.Badges) >= n }
}

// CampaignMilestones is ordered from a fresh overworld to durable Hall of
// Fame completion. A journey verifier reports the first missing milestone,
// which keeps a long Yellow run diagnosable instead of collapsing to one final
// "Champion not reached" assertion.
func CampaignMilestones() []Milestone {
	return []Milestone{
		{
			ID: "fresh_overworld", Description: "fresh game reached controllable overworld",
			Reached: func(obs game.ProfileObservation) bool {
				return obs.Controllable && !obs.InBattle && obs.NativeMapID == 0x26
			},
		},
		{ID: "starter", Description: "Oak gave Pikachu", Reached: progress(yellowprofile.ProgressYellowStarterReceived)},
		{ID: "lab_rival", Description: "Oak's Lab rival battle resolved", Reached: progress(yellowprofile.ProgressYellowLabRivalResolved)},
		{ID: "pokedex", Description: "Pokédex acquired", Reached: progress(gen1.ProgressPokedexAcquired)},
		{ID: "badge_1", Description: "first badge acquired", Reached: badgeCount(1)},
		{ID: "mt_moon_exit", Description: "Mt. Moon fossil and Jessie/James exit resolved", Reached: progress(yellowprofile.ProgressYellowMtMoonExitResolved)},
		{ID: "badge_2", Description: "second badge acquired", Reached: badgeCount(2)},
		{ID: "badge_3", Description: "third badge acquired", Reached: badgeCount(3)},
		{ID: "rocket_jessie_james", Description: "Rocket Hideout Jessie/James defeated", Reached: progress(yellowprofile.ProgressYellowRocketJessieJamesDefeated)},
		{ID: "badge_4", Description: "fourth badge acquired", Reached: badgeCount(4)},
		{ID: "tower_jessie_james", Description: "Pokémon Tower Jessie/James defeated", Reached: progress(yellowprofile.ProgressYellowTowerJessieJamesDefeated)},
		{ID: "poke_flute", Description: "Poké Flute acquired", Reached: progress(gen1.ProgressPokeFluteAcquired)},
		{ID: "fuchsia", Description: "Fuchsia Surf/Strength progression complete", Reached: progress(gen1.ProgressFuchsiaProgressionComplete)},
		{ID: "badge_5", Description: "fifth badge acquired", Reached: badgeCount(5)},
		{ID: "badge_6", Description: "sixth badge acquired", Reached: badgeCount(6)},
		{ID: "silph_jessie_james", Description: "Silph Co. Jessie/James defeated", Reached: progress(yellowprofile.ProgressYellowSilphJessieJamesDefeated)},
		{ID: "silph_rescue", Description: "Silph Co. rescue complete", Reached: progress(gen1.ProgressSilphRescueComplete)},
		{ID: "badge_7", Description: "seventh badge acquired", Reached: badgeCount(7)},
		{ID: "badge_8", Description: "all eight badges acquired", Reached: badgeCount(8)},
		{ID: "route_23", Description: "Route 23 badge checks complete", Reached: progress(gen1.ProgressRoute23BadgeChecks)},
		{ID: "victory_road", Description: "Victory Road cleared", Reached: progress(gen1.ProgressVictoryRoadCleared)},
		{ID: "league_started", Description: "Pokémon League challenge started", Reached: progress(gen1.ProgressLeagueChallengeStarted)},
		{ID: "champion", Description: "Champion rival defeated", Reached: progress(gen1.ProgressLeagueChampionDefeated)},
		{ID: "hall_of_fame", Description: "durable main-story Hall of Fame state", Reached: progress(gen1.ProgressMainStoryComplete)},
	}
}

type Status struct {
	Reached int
	Total   int
	Next    *Milestone
}

// Evaluate reports the longest reached prefix. Qualification is deliberately
// prefix-based: if an earlier required milestone is absent, later state does
// not hide the hole.
func Evaluate(obs game.ProfileObservation) Status {
	milestones := CampaignMilestones()
	status := Status{Total: len(milestones)}
	for i := range milestones {
		if !milestones[i].Reached(obs) {
			next := milestones[i]
			status.Next = &next
			return status
		}
		status.Reached++
	}
	return status
}

func (s Status) String() string {
	if s.Next == nil {
		return fmt.Sprintf("%d/%d milestones complete", s.Reached, s.Total)
	}
	return fmt.Sprintf("%d/%d milestones complete; next=%s (%s)", s.Reached, s.Total, s.Next.ID, s.Next.Description)
}
