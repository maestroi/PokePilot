package agent

import "github.com/maestroi/pokepilot/gen1"

// Yellow's League teams have the same maximum levels as the supported Red
// cartridge for Lorelei through the Champion (56, 58, 60, 62, 65). Keep the
// facts Yellow-owned even though they map to the same portable readiness scale.
func yellowLeagueChallengeProfiles() []CatalogChallengeProfile {
	specs := []struct {
		progress ProgressID
		maxLevel int
	}{
		{gen1.ProgressLeagueLoreleiDefeated, 56},
		{gen1.ProgressLeagueBrunoDefeated, 58},
		{gen1.ProgressLeagueAgathaDefeated, 60},
		{gen1.ProgressLeagueLanceDefeated, 62},
		{gen1.ProgressLeagueChampionDefeated, 65},
	}
	out := make([]CatalogChallengeProfile, 0, len(specs))
	for _, spec := range specs {
		out = append(out, CatalogChallengeProfile{
			Objective: (Objective{Kind: KindProgress, Progress: spec.progress}).Key(),
			Readiness: ChallengeReadinessProfile{MinimumReadiness: spec.maxLevel * 4},
		})
	}
	return out
}
