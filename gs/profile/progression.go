package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	ProgressStarterReceived          game.ProgressID = "gs_starter_received"
	ProgressMysteryEggReceived       game.ProgressID = "gs_mystery_egg_received"
	ProgressPokedexAcquired          game.ProgressID = "gs_pokedex_acquired"
	ProgressCherrygroveRivalResolved game.ProgressID = "gs_cherrygrove_rival_resolved"
	ProgressRivalNamed               game.ProgressID = "gs_rival_named"
	ProgressMysteryEggReturned       game.ProgressID = "gs_mystery_egg_returned"
	ProgressSproutTowerCleared       game.ProgressID = "gs_sprout_tower_cleared"
	ProgressZephyrBadgeEarned        game.ProgressID = "gs_zephyr_badge_earned"
	ProgressTogepiEggReceived        game.ProgressID = "gs_togepi_egg_received"
	ProgressSlowpokeWellCleared      game.ProgressID = "gs_slowpoke_well_cleared"
	ProgressHiveBadgeEarned          game.ProgressID = "gs_hive_badge_earned"
	ProgressAzaleaRivalResolved      game.ProgressID = "gs_azalea_rival_resolved"
	ProgressFarfetchdHerded          game.ProgressID = "gs_farfetchd_herded"
	ProgressHM01CutAcquired          game.ProgressID = "gs_hm01_cut_acquired"
	ProgressTM02HeadbuttAcquired     game.ProgressID = "gs_tm02_headbutt_acquired"
	ProgressPlainBadgeEarned         game.ProgressID = "gs_plain_badge_earned"
	// ProgressSupportedFrontier is the moving completion marker for experimental
	// Gen-II runs. Keep it tied to the furthest progression boundary the GS
	// objective adapter can execute end-to-end; advancing Gen II moves this one
	// semantic fact instead of changing every launch surface and saved preset.
	ProgressSupportedFrontier game.ProgressID = "gs_supported_frontier"
)

const (
	eventGotPokemonFromElm      uint16 = 26
	eventGotMysteryEggMrPokemon uint16 = 30
	eventGaveMysteryEggToElm    uint16 = 31
	// pret/pokegold constants/event_flags.asm at gs/data.SourceRevision.
	// Keep these zero-based event indices pinned: event flags are addressed
	// directly as EventFlags + event/8 below.
	eventGotHM05Flash             uint16 = 20
	eventGotHM01Cut               uint16 = 16
	eventGotTM02Headbutt          uint16 = 95
	eventHerdedFarfetchd          uint16 = 41
	eventGotTogepiEggFromElmsAide uint16 = 45
	eventClearedSlowpokeWell      uint16 = 43
	eventRivalAzaleaTown          uint16 = 1727
	statusFlagsPokedexMask               = 1 << 0
	johtoBadgeZephyrMask                 = 1 << 0
	johtoBadgeHiveMask                   = 1 << 1
	johtoBadgePlainMask                  = 1 << 2

	sceneCherrygroveNoop     = 0
	sceneAzaleaTownNoop      = 0
	sceneMrPokemonsHouseNoop = 1
	sceneElmsLabNoop         = 2
)

type earlyStoryFacts struct {
	StarterReceived          bool
	MysteryEggReceived       bool
	PokedexAcquired          bool
	MrPokemonVisitComplete   bool
	CherrygroveRivalResolved bool
	RivalNamed               bool
	MysteryEggReturned       bool
}

func hasGSEvent(reader game.MemoryReader, event uint16) bool {
	if reader == nil {
		return false
	}
	return reader.Peek8(sym.EventFlags+event/8)&byte(1<<uint(event%8)) != 0
}

func decodeEarlyStory(reader game.MemoryReader) earlyStoryFacts {
	if reader == nil {
		return earlyStoryFacts{}
	}
	starter := hasGSEvent(reader, eventGotPokemonFromElm)
	egg := hasGSEvent(reader, eventGotMysteryEggMrPokemon)
	pokedex := reader.Peek8(sym.StatusFlags)&statusFlagsPokedexMask != 0
	mrPokemonComplete := egg && pokedex && reader.Peek8(sym.MrPokemonsHouseSceneID) == sceneMrPokemonsHouseNoop
	rivalResolved := mrPokemonComplete && reader.Peek8(sym.CherrygroveCitySceneID) == sceneCherrygroveNoop
	rivalNamed := rivalResolved && reader.Peek8(sym.ElmsLabSceneID) == sceneElmsLabNoop
	return earlyStoryFacts{
		StarterReceived:          starter,
		MysteryEggReceived:       egg,
		PokedexAcquired:          pokedex,
		MrPokemonVisitComplete:   mrPokemonComplete,
		CherrygroveRivalResolved: rivalResolved,
		RivalNamed:               rivalNamed,
		MysteryEggReturned:       hasGSEvent(reader, eventGaveMysteryEggToElm),
	}
}

func projectEarlyStory(reader game.MemoryReader) game.ProgressState {
	f := decodeEarlyStory(reader)
	return game.ProgressState{
		{ID: ProgressStarterReceived, Complete: f.StarterReceived},
		{ID: ProgressMysteryEggReceived, Complete: f.MysteryEggReceived},
		{ID: ProgressPokedexAcquired, Complete: f.PokedexAcquired},
		{ID: ProgressCherrygroveRivalResolved, Complete: f.CherrygroveRivalResolved},
		{ID: ProgressRivalNamed, Complete: f.RivalNamed},
		{ID: ProgressMysteryEggReturned, Complete: f.MysteryEggReturned},
		{ID: ProgressSproutTowerCleared, Complete: hasGSEvent(reader, eventGotHM05Flash)},
		{ID: ProgressZephyrBadgeEarned, Complete: reader.Peek8(sym.JohtoBadges)&johtoBadgeZephyrMask != 0},
		{ID: ProgressTogepiEggReceived, Complete: hasGSEvent(reader, eventGotTogepiEggFromElmsAide)},
		{ID: ProgressSlowpokeWellCleared, Complete: hasGSEvent(reader, eventClearedSlowpokeWell)},
		{ID: ProgressHiveBadgeEarned, Complete: reader.Peek8(sym.JohtoBadges)&johtoBadgeHiveMask != 0},
		{
			ID: ProgressAzaleaRivalResolved,
			// EVENT_RIVAL_AZALEA_TOWN is set before startbattle, so it is not
			// sufficient after a loss. The post-win script resets the scene to
			// NOOP; requiring both facts makes this durable and retry-safe.
			Complete: reader.Peek8(sym.JohtoBadges)&johtoBadgeHiveMask != 0 &&
				hasGSEvent(reader, eventRivalAzaleaTown) &&
				reader.Peek8(sym.AzaleaTownSceneID) == sceneAzaleaTownNoop,
		},
		{ID: ProgressFarfetchdHerded, Complete: hasGSEvent(reader, eventHerdedFarfetchd)},
		{ID: ProgressHM01CutAcquired, Complete: hasGSEvent(reader, eventGotHM01Cut)},
		{ID: ProgressTM02HeadbuttAcquired, Complete: hasGSEvent(reader, eventGotTM02Headbutt)},
		{ID: ProgressPlainBadgeEarned, Complete: reader.Peek8(sym.JohtoBadges)&johtoBadgePlainMask != 0},
		{ID: ProgressSupportedFrontier, Complete: reader.Peek8(sym.JohtoBadges)&johtoBadgePlainMask != 0},
	}
}

// DecodeFirstBadgeProgress exposes the durable early-Johto story boundaries
// needed by the early-Johto executors without
// leaking event numbers or badge bit positions into agent policy.
func (*Profile) DecodeFirstBadgeProgress(reader game.MemoryReader) game.ProgressState {
	return projectEarlyStory(reader)
}
