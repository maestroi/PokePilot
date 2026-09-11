package agent

import gameruntime "github.com/maestroi/pokepilot/game"

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
	ProgressSaffronGateOpen        ProgressID = "saffron_gate_open"
	ProgressCardKeyOwned           ProgressID = "card_key_owned"
	ProgressSilphCoCleared         ProgressID = "silph_co_cleared"
	ProgressMansionSwitchOn        ProgressID = "mansion_switch_on"
	ProgressSecretKeyOwned         ProgressID = "secret_key_owned"
	ProgressViridianGymOpen        ProgressID = "viridian_gym_open"
	ProgressRoute22RivalResolved   ProgressID = "route_22_rival_resolved"
	ProgressRoute23BadgeChecks     ProgressID = "route_23_badge_checks"
	ProgressLeagueChallengeStarted ProgressID = "league_challenge_started"
	ProgressLeagueChampionDefeated ProgressID = "league_champion_defeated"
	ProgressMainStoryComplete      ProgressID = "main_story_complete"
)

const (
	StarterCharmander SpeciesID = "charmander"
	StarterSquirtle   SpeciesID = "squirtle"
	StarterBulbasaur  SpeciesID = "bulbasaur"
)
