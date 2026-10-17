package agent

import (
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

var gsSixBadgeFacts = []game.ProgressID{
	gsprofile.ProgressZephyrBadgeEarned,
	gsprofile.ProgressHiveBadgeEarned,
	gsprofile.ProgressPlainBadgeEarned,
	gsprofile.ProgressFogBadgeEarned,
	gsprofile.ProgressStormBadgeEarned,
	gsprofile.ProgressMineralBadgeEarned,
}

// gsPostMineralCaptureEligible accepts only the FIRST authentic sixth-badge
// transaction; an arbitrary loaded checkpoint with six badges is not enough.
func gsPostMineralCaptureEligible(o Objective, before, after Observation) bool {
	if o.Kind != KindProgress || o.Progress != gsprofile.ProgressMineralBadgeEarned ||
		(before.GameID != gsprofile.GoldGameID && before.GameID != gsprofile.SilverGameID) ||
		before.GameID != after.GameID ||
		!before.Controllable || before.InBattle || !after.Controllable || after.InBattle ||
		before.Story.Has(gsprofile.ProgressMineralBadgeEarned) ||
		!after.Story.Has(gsprofile.ProgressMineralBadgeEarned) ||
		after.Story.Has(gsprofile.ProgressRedGyaradosDefeated) ||
		after.Story.Has(gsprofile.ProgressRocketHideoutCleared) ||
		after.Story.Has(gsprofile.ProgressGlacierBadgeEarned) {
		return false
	}
	for _, badge := range gsSixBadgeFacts[:len(gsSixBadgeFacts)-1] {
		if !before.Story.Has(badge) || !after.Story.Has(badge) {
			return false
		}
	}
	return true
}
