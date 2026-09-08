package agent

import (
	"fmt"
	"strings"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func semanticPlace(name string) PlaceID {
	return gameruntime.CanonicalID(name)
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

func machineItemID(machine rom.Machine) ItemID {
	if machine.HM {
		return ItemID(fmt.Sprintf("hm%02d", machine.Number-rom.NumTMs))
	}
	return ItemID(fmt.Sprintf("tm%02d", machine.Number))
}

// Starter remains the existing Red opening-story enum until #137 replaces
// Red-specific progression verbs. Keeping its validation here avoids widening
// #136 beyond planner species/item/location identity.
func redStarter(id skill.Starter) (skill.Starter, bool) {
	if id > skill.StarterBulbasaur {
		return 0, false
	}
	return id, true
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
