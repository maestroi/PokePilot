package agent

import (
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
)

// Re-export the portable semantic vocabulary at the planner boundary so
// callers constructing objectives/fixtures do not need to know which concrete
// game adapter will execute them.
type (
	PlaceID       = gameruntime.PlaceID
	SpeciesID     = gameruntime.SpeciesID
	ItemID        = gameruntime.ItemID
	CapabilityID  = gameruntime.CapabilityID
	ProgressID    = gameruntime.ProgressID
	ProgressFact  = gameruntime.ProgressFact
	ProgressState = gameruntime.ProgressState
)

// Current Red progression concepts are expressed as semantic facts rather
// than red/state fields. They remain ordinary IDs so another game may expose a
// completely different set without changing ProgressState itself. #137 moves
// Red-specific progression planning behind the adapter.
const (
	ProgressSaffronGateOpen        ProgressID = gen1.ProgressSaffronGateOpen
	ProgressCardKeyOwned           ProgressID = gen1.ProgressCardKeyOwned
	ProgressSilphCoCleared         ProgressID = gen1.ProgressSilphCoCleared
	ProgressMansionSwitchOn        ProgressID = gen1.ProgressMansionSwitchOn
	ProgressSecretKeyOwned         ProgressID = gen1.ProgressSecretKeyOwned
	ProgressViridianGymOpen        ProgressID = gen1.ProgressViridianGymOpen
	ProgressRoute22RivalResolved   ProgressID = gen1.ProgressRoute22RivalResolved
	ProgressRoute23BadgeChecks     ProgressID = gen1.ProgressRoute23BadgeChecks
	ProgressLeagueChallengeStarted ProgressID = gen1.ProgressLeagueChallengeStarted
	ProgressLeagueChampionDefeated ProgressID = gen1.ProgressLeagueChampionDefeated
	ProgressMainStoryComplete      ProgressID = gen1.ProgressMainStoryComplete
)

const (
	StarterCharmander SpeciesID = "charmander"
	StarterSquirtle   SpeciesID = "squirtle"
	StarterBulbasaur  SpeciesID = "bulbasaur"
)
