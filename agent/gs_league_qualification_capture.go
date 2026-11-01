package agent

import (
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

// gsLeagueCheckpointBoundary names the first successfully completed milestone
// that can seed a deterministic next-leg ROM replay. The cartridge owns the
// event flags, and the run loop calls this only after a committed objective.
// A pre-existing save that merely contains the flag is not evidence of a new
// transaction. This is intentionally separate from the normal checkpoint
// ring: its entries are immutable qualification fixtures.
type gsLeagueCheckpointBoundary struct {
	progress  ProgressID
	previous  ProgressID
	directory string
	mapName   string
}

var gsLeagueCheckpointBoundaries = [...]gsLeagueCheckpointBoundary{
	{gsprofile.ProgressRisingBadgeEarned, gsprofile.ProgressClairDefeated, "post-rising", "DRAGONS_DEN_B1F"},
	{gsprofile.ProgressIndigoPlateauReached, gsprofile.ProgressRisingBadgeEarned, "post-indigo", "ROUTE_23"},
	{gsprofile.ProgressLeagueWillDefeated, gsprofile.ProgressIndigoPlateauReached, "post-will", "WILLS_ROOM"},
	{gsprofile.ProgressLeagueKogaDefeated, gsprofile.ProgressLeagueWillDefeated, "post-koga", "KOGAS_ROOM"},
	{gsprofile.ProgressLeagueBrunoDefeated, gsprofile.ProgressLeagueKogaDefeated, "post-bruno", "BRUNOS_ROOM"},
	{gsprofile.ProgressLeagueKarenDefeated, gsprofile.ProgressLeagueBrunoDefeated, "post-karen", "KARENS_ROOM"},
}

// gsLeagueCaptureEligible allows the first *observed* successful objective
// transition at a stable, controllable boundary. A saved post-Hall-of-Fame
// rematch resets the Elite Four victory bits; it must never overwrite or
// establish a first-time pre-Champion qualification fixture.
func gsLeagueCaptureEligible(o Objective, before, after Observation) (gsLeagueCheckpointBoundary, bool) {
	if o.Kind != KindProgress ||
		(before.GameID != gsprofile.GoldGameID && before.GameID != gsprofile.SilverGameID) ||
		before.GameID != after.GameID ||
		!before.Controllable || before.InBattle ||
		!after.Controllable || after.InBattle ||
		before.Story.Has(gsprofile.ProgressLeagueHallOfFameEntered) ||
		after.Story.Has(gsprofile.ProgressLeagueHallOfFameEntered) {
		return gsLeagueCheckpointBoundary{}, false
	}
	for _, stage := range gsLeagueCheckpointBoundaries {
		if o.Progress != stage.progress {
			continue
		}
		if before.Story.Has(stage.progress) || !after.Story.Has(stage.progress) ||
			!before.Story.Has(stage.previous) || !after.Story.Has(stage.previous) {
			return gsLeagueCheckpointBoundary{}, false
		}
		return stage, true
	}
	return gsLeagueCheckpointBoundary{}, false
}
