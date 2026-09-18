package gen1

import "github.com/maestroi/pokepilot/game"

// Progress IDs shared by the supported Kanto Generation-I campaigns. Concrete
// games own how these semantic facts are derived from native events/RAM.
const (
	ProgressMtMoonFossilAcquired       game.ProgressID = "mt_moon_fossil_acquired"
	ProgressPokedexAcquired            game.ProgressID = "pokedex_acquired"
	ProgressSSTicketAcquired           game.ProgressID = "ss_ticket_acquired"
	ProgressHM01Acquired               game.ProgressID = "hm01_acquired"
	ProgressBicycleAcquired            game.ProgressID = "bicycle_acquired"
	ProgressThunderBadge               game.ProgressID = "thunder_badge"
	ProgressPostSurgeLavenderReached   game.ProgressID = "post_surge_lavender_reached"
	ProgressPostSurgeCeladonReady      game.ProgressID = "post_surge_celadon_ready"
	ProgressRainbowBadge               game.ProgressID = "rainbow_badge"
	ProgressSilphScopeAcquired         game.ProgressID = "silph_scope_acquired"
	ProgressPokeFluteAcquired          game.ProgressID = "poke_flute_acquired"
	ProgressFuchsiaProgressionComplete game.ProgressID = "fuchsia_progression_complete"
	ProgressSaffronGateOpen            game.ProgressID = "saffron_gate_open"
	ProgressCardKeyOwned               game.ProgressID = "card_key_owned"
	ProgressSilphCoCleared             game.ProgressID = "silph_co_cleared"
	ProgressSilphRescueComplete        game.ProgressID = "silph_rescue_complete"
	ProgressMansionSwitchOn            game.ProgressID = "mansion_switch_on"
	ProgressSecretKeyOwned             game.ProgressID = "secret_key_owned"
	ProgressViridianGymOpen            game.ProgressID = "viridian_gym_open"
	ProgressRoute22RivalResolved       game.ProgressID = "route_22_rival_resolved"
	ProgressRoute23BadgeChecks         game.ProgressID = "route_23_badge_checks"
	ProgressVictoryRoadCleared         game.ProgressID = "victory_road_cleared"
	ProgressLeagueChallengeStarted     game.ProgressID = "league_challenge_started"
	ProgressLeagueChampionDefeated     game.ProgressID = "league_champion_defeated"
	ProgressMainStoryComplete          game.ProgressID = "main_story_complete"
	ProgressVolcanoBadge               game.ProgressID = "volcano_badge"
	ProgressEarthBadge                 game.ProgressID = "earth_badge"
	ProgressIndigoPlateauReady         game.ProgressID = "indigo_plateau_ready"
)
