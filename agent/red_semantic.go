package agent

import (
	"strings"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func semanticPlace(name string) PlaceID {
	return PlaceID(gameruntime.CanonicalID(name))
}

func semanticLocation(mapName string) PlaceID {
	return semanticPlace(strings.ReplaceAll(mapName, "_", " "))
}

func semanticSpecies(name string) (SpeciesID, bool) {
	name = gameruntime.CanonicalID(name)
	if _, ok := speciesTable[name]; !ok {
		return "", false
	}
	return SpeciesID(name), true
}

func semanticSpeciesFromRed(raw uint8) SpeciesID {
	if name, ok := SpeciesName(raw); ok {
		return SpeciesID(name)
	}
	return SpeciesID("unknown")
}

func redSpeciesID(id SpeciesID) (uint8, bool) {
	raw, ok := speciesTable[gameruntime.CanonicalID(string(id))]
	return raw, ok
}

func semanticItem(name string) (ItemID, bool) {
	name = gameruntime.CanonicalID(name)
	if _, ok := itemTable[name]; !ok {
		return "", false
	}
	return ItemID(name), true
}

func semanticItemFromRed(raw uint8) ItemID {
	if name, ok := ItemName(raw); ok {
		return ItemID(name)
	}
	return ItemID("unknown")
}

func redItemID(id ItemID) (uint8, bool) {
	raw, ok := itemTable[gameruntime.CanonicalID(string(id))]
	return raw, ok
}

func redStarter(id SpeciesID) (skill.Starter, bool) {
	switch SpeciesID(gameruntime.CanonicalID(string(id))) {
	case StarterCharmander:
		return skill.StarterCharmander, true
	case StarterSquirtle:
		return skill.StarterSquirtle, true
	case StarterBulbasaur:
		return skill.StarterBulbasaur, true
	default:
		return 0, false
	}
}

func redProgressState(f state.StoryFacts) ProgressState {
	return ProgressState{
		{ID: ProgressSaffronGateOpen, Complete: f.SaffronGateOpen},
		{ID: ProgressCardKeyOwned, Complete: f.CardKeyOwned},
		{ID: ProgressSilphCoCleared, Complete: f.SilphCoCleared},
		{ID: ProgressMansionSwitchOn, Complete: f.MansionSwitchOn},
		{ID: ProgressSecretKeyOwned, Complete: f.SecretKeyOwned},
		{ID: ProgressViridianGymOpen, Complete: f.ViridianGymOpen},
		{ID: ProgressRoute22RivalResolved, Complete: f.Route22RivalResolved},
		{ID: ProgressRoute23BadgeChecks, Complete: f.Route23BadgeChecksComplete, Value: f.Route23BadgeChecksPassed},
		{ID: ProgressLeagueChallengeStarted, Complete: f.LeagueChallengeStarted},
		{ID: ProgressLeagueChampionDefeated, Complete: f.LeagueChampionDefeated},
	}
}
