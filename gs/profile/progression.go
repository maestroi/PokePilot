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
)

const (
	eventGotPokemonFromElm      uint16 = 26
	eventGotMysteryEggMrPokemon uint16 = 30
	eventGaveMysteryEggToElm    uint16 = 31
	eventGotHM05Flash            uint16 = 21
	statusFlagsPokedexMask             = 1 << 0
	johtoBadgeZephyrMask               = 1 << 0

	sceneCherrygroveNoop     = 0
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
	}
}
