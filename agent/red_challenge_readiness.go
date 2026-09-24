package agent

import "github.com/maestroi/pokepilot/red/state"

// Red/Blue boss level ceilings below come from the vendored Gen I trainer
// tables (pokered/data/trainers/parties.asm). Generic readiness never knows
// these names or values; the adapter translates the game's documented challenge
// facts into the portable weighted party-readiness scale.
func redReadinessFloor(maxEnemyLevel int) int {
	if maxEnemyLevel <= 0 {
		return 0
	}
	return maxEnemyLevel * 4
}

func redGymReadinessProfile(badge state.Badge) ChallengeReadinessProfile {
	maxLevel := map[state.Badge]int{
		state.BadgeBoulder: 14,
		state.BadgeCascade: 21,
		state.BadgeThunder: 24,
		state.BadgeRainbow: 29,
		state.BadgeSoul:    43,
		state.BadgeMarsh:   43,
		state.BadgeVolcano: 47,
		state.BadgeEarth:   50,
	}[badge]
	return ChallengeReadinessProfile{MinimumReadiness: redReadinessFloor(maxLevel)}
}

func redProgressionChallengeProfiles() []CatalogChallengeProfile {
	specs := []struct {
		objective Objective
		maxLevel  int
		// League stages: fights left through the Champion with no Center
		// between them. A blackout anywhere returns to the lobby entry.
		leagueFights int
	}{
		// S.S. Anne rival.
		{Objective{Kind: KindProgress, Progress: redProgressHM01Acquired}, 20, 0},
		// Rocket Hideout Giovanni.
		{Objective{Kind: KindProgress, Progress: redProgressSilphScopeAcquired}, 29, 0},
		// Pokemon Tower's rival/Rocket combat leg.
		{Objective{Kind: KindProgress, Progress: redProgressPokeFluteAcquired}, 30, 0},
		// Fuchsia progression includes Koga.
		{Objective{Kind: KindProgress, Progress: redProgressFuchsiaProgressionComplete}, 43, 0},
		// Silph Co includes the late rival and Giovanni.
		{Objective{Kind: KindProgress, Progress: redProgressSilphRescueComplete}, 41, 0},
		// Badge progression objectives whose executor owns the leader battle.
		{Objective{Kind: KindProgress, Progress: redProgressBoulderBadge}, 14, 0},
		{Objective{Kind: KindProgress, Progress: redProgressThunderBadge}, 24, 0},
		{Objective{Kind: KindProgress, Progress: redProgressRainbowBadge}, 29, 0},
		{Objective{Kind: KindProgress, Progress: redProgressVolcanoBadge}, 47, 0},
		{Objective{Kind: KindProgress, Progress: redProgressEarthBadge}, 50, 0},
		// Final Route 22 rival before the League approach.
		{Objective{Kind: KindProgress, Progress: ProgressRoute22RivalResolved}, 53, 0},
		// Elite Four + Champion staged transactions. Entering commits to the
		// whole gauntlet, so the entry is gated by its hardest member.
		{Objective{Kind: KindProgress, Progress: ProgressLeagueChallengeStarted}, 65, 5},
		{Objective{Kind: KindProgress, Progress: redProgressLeagueLoreleiDefeated}, 56, 5},
		{Objective{Kind: KindProgress, Progress: redProgressLeagueBrunoDefeated}, 58, 4},
		{Objective{Kind: KindProgress, Progress: redProgressLeagueAgathaDefeated}, 60, 3},
		{Objective{Kind: KindProgress, Progress: redProgressLeagueLanceDefeated}, 62, 2},
		{Objective{Kind: KindProgress, Progress: ProgressLeagueChampionDefeated}, 65, 1},
	}
	out := make([]CatalogChallengeProfile, 0, len(specs))
	for _, spec := range specs {
		profile := CatalogChallengeProfile{
			Objective: spec.objective.Key(),
			Readiness: ChallengeReadinessProfile{MinimumReadiness: redReadinessFloor(spec.maxLevel)},
		}
		if spec.leagueFights > 0 {
			profile.Chain = "pokemon_league"
			profile.Readiness.RecoveryFights = spec.leagueFights
			// ponytail: support floor is max enemy level - 15, a coarse depth
			// check; replace with per-member matchup scoring if it over-trains.
			profile.Readiness.MinimumSupportLevel = spec.maxLevel - 15
		}
		out = append(out, profile)
	}
	return out
}
