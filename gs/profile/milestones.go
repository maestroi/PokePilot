package profile

import "github.com/maestroi/pokepilot/game"

// gsMilestones are the Gold/Silver major story beats shown in live telemetry,
// in story order. Badges stay on Observation.Badges. "Red" is the end of the
// game; "Hall of Fame" is only the Johto League.
var gsMilestones = []struct {
	id    game.ProgressID
	label string
}{
	{ProgressPokedexAcquired, "Pokédex"},
	{ProgressHM01CutAcquired, "HM01 Cut"},
	{ProgressHM03SurfAcquired, "HM03 Surf"},
	{ProgressHM04StrengthAcquired, "HM04 Strength"},
	{ProgressHM02FlyAcquired, "HM02 Fly"},
	{ProgressRocketHideoutCleared, "Rocket Hideout"},
	{ProgressRadioTowerCleared, "Radio Tower"},
	{ProgressIcePathWaterfallAcquired, "HM07 Waterfall"},
	{ProgressLeagueLanceDefeated, "Champion"},
	{ProgressLeagueHallOfFameEntered, "Hall of Fame"},
	{ProgressSSTicketReceived, "S.S. Ticket"},
	{ProgressKantoPowerRestored, "Power Plant"},
	{ProgressKantoRailPassReceived, "Magnet Train Pass"},
	{ProgressKantoSnorlaxCleared, "Snorlax"},
	{ProgressMtSilverOpened, "Mt. Silver"},
	{ProgressMtSilverRedDefeated, "Red"},
}

func milestoneLabels(story game.ProgressState) []string {
	var out []string
	for _, m := range gsMilestones {
		// Lance's battle flag resets on each new League attempt; once the
		// durable Hall of Fame is set, Champion stays credited.
		if story.Has(m.id) || (m.id == ProgressLeagueLanceDefeated && story.Has(ProgressLeagueHallOfFameEntered)) {
			out = append(out, m.label)
		}
	}
	return out
}
