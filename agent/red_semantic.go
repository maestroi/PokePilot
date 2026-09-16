package agent

import (
	"fmt"
	"strings"

	gameruntime "github.com/maestroi/pokepilot/game"
	reddata "github.com/maestroi/pokepilot/red/data"
	redprofile "github.com/maestroi/pokepilot/red/profile"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// Keep the existing agent names as aliases while progression execution is
// migrated. The concrete vocabulary is owned by the Red profile where it
// already exists; League room facts remain adapter-local until that migration
// is complete.
const (
	redProgressMtMoonFossilAcquired       ProgressID = redprofile.ProgressMtMoonFossilAcquired
	redProgressPokedexAcquired            ProgressID = redprofile.ProgressPokedexAcquired
	redProgressSSTicketAcquired           ProgressID = redprofile.ProgressSSTicketAcquired
	redProgressHM01Acquired               ProgressID = redprofile.ProgressHM01Acquired
	redProgressBicycleAcquired            ProgressID = redprofile.ProgressBicycleAcquired
	redProgressThunderBadge               ProgressID = redprofile.ProgressThunderBadge
	redProgressPostSurgeLavenderReached   ProgressID = redprofile.ProgressPostSurgeLavenderReached
	redProgressPostSurgeCeladonReady      ProgressID = redprofile.ProgressPostSurgeCeladonReady
	redProgressRainbowBadge               ProgressID = redprofile.ProgressRainbowBadge
	redProgressSilphScopeAcquired         ProgressID = redprofile.ProgressSilphScopeAcquired
	redProgressPokeFluteAcquired          ProgressID = redprofile.ProgressPokeFluteAcquired
	redProgressFuchsiaProgressionComplete ProgressID = redprofile.ProgressFuchsiaProgressionComplete
	redProgressSilphRescueComplete        ProgressID = redprofile.ProgressSilphRescueComplete
	redProgressVolcanoBadge               ProgressID = redprofile.ProgressVolcanoBadge
	redProgressEarthBadge                 ProgressID = redprofile.ProgressEarthBadge
	redProgressVictoryRoadCleared         ProgressID = redprofile.ProgressVictoryRoadCleared
	redProgressIndigoPlateauReady         ProgressID = redprofile.ProgressIndigoPlateauReady
	redProgressLeagueLoreleiDefeated      ProgressID = "league_lorelei_defeated"
	redProgressLeagueBrunoDefeated        ProgressID = "league_bruno_defeated"
	redProgressLeagueAgathaDefeated       ProgressID = "league_agatha_defeated"
	redProgressLeagueLanceDefeated        ProgressID = "league_lance_defeated"

	redIndigoPlateauMap      uint8 = 0x09
	redIndigoPlateauLobbyMap uint8 = 0xAE
)

func semanticPlace(name string) PlaceID {
	return gameruntime.CanonicalID(name)
}

func semanticLocation(mapName string) PlaceID {
	return semanticPlace(strings.ReplaceAll(mapName, "_", " "))
}

func semanticSpecies(name string) (SpeciesID, bool) {
	id := SpeciesID(gameruntime.CanonicalID(name))
	if _, ok := reddata.SpeciesRaw(id); !ok {
		return "", false
	}
	return id, true
}

func semanticSpeciesFromRed(raw uint8) SpeciesID {
	if id, ok := reddata.Species(raw); ok {
		return SpeciesID(id)
	}
	return SpeciesID("unknown")
}

func redSpeciesID(id SpeciesID) (uint8, bool) {
	return reddata.SpeciesRaw(id)
}

func semanticItem(name string) (ItemID, bool) {
	id := ItemID(gameruntime.CanonicalID(name))
	if _, ok := reddata.ItemRaw(id); !ok {
		return "", false
	}
	return id, true
}

func semanticItemFromRed(raw uint8) ItemID {
	if id, ok := reddata.Item(raw); ok {
		return ItemID(id)
	}
	return ItemID("unknown")
}

func redItemID(id ItemID) (uint8, bool) {
	return reddata.ItemRaw(id)
}

func machineItemID(machine rom.Machine) ItemID {
	if machine.HM {
		return ItemID(fmt.Sprintf("hm%02d", machine.Number-rom.NumTMs))
	}
	return ItemID(fmt.Sprintf("tm%02d", machine.Number))
}

func redStarter(id skill.Starter) (skill.Starter, bool) {
	if id > skill.StarterBulbasaur {
		return 0, false
	}
	return id, true
}

func appendRedLeagueProgress(progress ProgressState, f state.StoryFacts) ProgressState {
	return append(progress,
		ProgressFact{ID: redProgressLeagueLoreleiDefeated, Complete: f.LeagueLoreleiDefeated},
		ProgressFact{ID: redProgressLeagueBrunoDefeated, Complete: f.LeagueBrunoDefeated},
		ProgressFact{ID: redProgressLeagueAgathaDefeated, Complete: f.LeagueAgathaDefeated},
		ProgressFact{ID: redProgressLeagueLanceDefeated, Complete: f.LeagueLanceDefeated},
	)
}

func redProgressState(f state.StoryFacts) ProgressState {
	return appendRedLeagueProgress(redprofile.ProjectStoryFacts(f), f)
}

func redProgressStateFromRAM(mem *state.Mem, _ state.InventoryState, f state.StoryFacts) ProgressState {
	return appendRedLeagueProgress(redprofile.ProjectStory(mem, f), f)
}
