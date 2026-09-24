package agent

import "github.com/maestroi/pokepilot/red/state"

// Red/Blue boss level ceilings below come from the vendored Gen I trainer
// tables (pokered/data/trainers/parties.asm). Generic readiness never knows
// these names or values; the adapter translates the game's documented challenge
// facts into the portable weighted party-readiness scale.
// redLeagueHealingStock is the HP healing carried into the Elite Four: two per
// chained fight is roughly one top-up per battle for the lead plus a spare.
// Money bounds what is actually bought.
const redLeagueHealingStock = 10

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
	}{
		// S.S. Anne rival.
		{Objective{Kind: KindProgress, Progress: redProgressHM01Acquired}, 20},
		// Rocket Hideout Giovanni.
		{Objective{Kind: KindProgress, Progress: redProgressSilphScopeAcquired}, 29},
		// Pokemon Tower's rival/Rocket combat leg.
		{Objective{Kind: KindProgress, Progress: redProgressPokeFluteAcquired}, 30},
		// Fuchsia progression includes Koga.
		{Objective{Kind: KindProgress, Progress: redProgressFuchsiaProgressionComplete}, 43},
		// Silph Co includes the late rival and Giovanni.
		{Objective{Kind: KindProgress, Progress: redProgressSilphRescueComplete}, 41},
		// Badge progression objectives whose executor owns the leader battle.
		{Objective{Kind: KindProgress, Progress: redProgressBoulderBadge}, 14},
		{Objective{Kind: KindProgress, Progress: redProgressThunderBadge}, 24},
		{Objective{Kind: KindProgress, Progress: redProgressRainbowBadge}, 29},
		{Objective{Kind: KindProgress, Progress: redProgressVolcanoBadge}, 47},
		{Objective{Kind: KindProgress, Progress: redProgressEarthBadge}, 50},
		// Final Route 22 rival before the League approach.
		{Objective{Kind: KindProgress, Progress: ProgressRoute22RivalResolved}, 53},
		// Elite Four + Champion staged transactions.
		{Objective{Kind: KindProgress, Progress: redProgressLeagueLoreleiDefeated}, 56},
		{Objective{Kind: KindProgress, Progress: redProgressLeagueBrunoDefeated}, 58},
		{Objective{Kind: KindProgress, Progress: redProgressLeagueAgathaDefeated}, 60},
		{Objective{Kind: KindProgress, Progress: redProgressLeagueLanceDefeated}, 62},
		{Objective{Kind: KindProgress, Progress: ProgressLeagueChampionDefeated}, 65},
	}
	out := make([]CatalogChallengeProfile, 0, len(specs)+1)
	for _, spec := range specs {
		out = append(out, CatalogChallengeProfile{
			Objective: spec.objective.Key(),
			Readiness: ChallengeReadinessProfile{MinimumReadiness: redReadinessFloor(spec.maxLevel)},
		})
	}
	// Committing to the League enters five chained fights with no way back to
	// the lobby Center, so the bag is the only healing. The lobby shop sits
	// right beside the commit point.
	out = append(out, CatalogChallengeProfile{
		Objective: Objective{Kind: KindProgress, Progress: ProgressLeagueChallengeStarted}.Key(),
		Readiness: ChallengeReadinessProfile{MinimumHealingStock: redLeagueHealingStock},
	})
	return out
}
