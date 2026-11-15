package agent

import (
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

// gsKantoCheckpointBoundary is an immutable first-clear checkpoint at a
// cartridge-observed story transition. These states are inputs for the next
// milestone, never synthetic shortcuts around an unfinished retail script.
type gsKantoCheckpointBoundary struct {
	progress  game.ProgressID
	previous  game.ProgressID
	directory string
	mapName   string
}

var gsKantoCheckpointBoundaries = [...]gsKantoCheckpointBoundary{
	{gsprofile.ProgressSSTicketReceived, gsprofile.ProgressLeagueHallOfFameEntered, "post-ticket", "ELMS_LAB"},
	{gsprofile.ProgressKantoFirstLanding, gsprofile.ProgressSSTicketReceived, "post-landing", "VERMILION_PORT"},
	{gsprofile.ProgressKantoThunderBadgeEarned, gsprofile.ProgressKantoFirstLanding, "post-thunder", "VERMILION_GYM"},
	{gsprofile.ProgressKantoMarshBadgeEarned, gsprofile.ProgressKantoThunderBadgeEarned, "post-marsh", "SAFFRON_GYM"},
	{gsprofile.ProgressKantoRainbowBadgeEarned, gsprofile.ProgressKantoMarshBadgeEarned, "post-rainbow", "CELADON_GYM"},
	{gsprofile.ProgressKantoSoulBadgeEarned, gsprofile.ProgressKantoRainbowBadgeEarned, "post-soul", "FUCHSIA_GYM"},
}

// gsKantoCaptureEligible accepts only a NEW verified progress transition
// after the real Hall-of-Fame event. Capturing a previously completed save,
// the ship interior, the wrong region, or an in-flight battle/cutscene would
// poison all later replays. The caller additionally checks the live native
// map before publishing the private, first-writer-wins save state.
func gsKantoCaptureEligible(o Objective, before, after Observation) (gsKantoCheckpointBoundary, bool) {
	if o.Kind != KindProgress ||
		(before.GameID != gsprofile.GoldGameID && before.GameID != gsprofile.SilverGameID) ||
		before.GameID != after.GameID ||
		!before.Controllable || before.InBattle ||
		!after.Controllable || after.InBattle ||
		!before.Story.Has(gsprofile.ProgressLeagueHallOfFameEntered) ||
		!after.Story.Has(gsprofile.ProgressLeagueHallOfFameEntered) {
		return gsKantoCheckpointBoundary{}, false
	}
	for _, stage := range gsKantoCheckpointBoundaries {
		if o.Progress != stage.progress {
			continue
		}
		if before.Story.Has(stage.progress) || !after.Story.Has(stage.progress) ||
			!before.Story.Has(stage.previous) || !after.Story.Has(stage.previous) {
			return gsKantoCheckpointBoundary{}, false
		}
		if stage.progress == gsprofile.ProgressSSTicketReceived {
			if after.Region != game.RegionJohto {
				return gsKantoCheckpointBoundary{}, false
			}
		} else if after.Region != game.RegionKanto {
			return gsKantoCheckpointBoundary{}, false
		}
		return stage, true
	}
	return gsKantoCheckpointBoundary{}, false
}
