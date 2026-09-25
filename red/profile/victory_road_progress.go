package profile

import (
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const profileRoute23Map uint8 = 0x22

// victoryRoadClearedForProgress turns the live final-switch event into the
// semantic stage fact used by the planner. Route 23 resets the cave's boulder
// events on load, so after a successful exit the authoritative evidence is the
// player's position north of the cave (or at Indigo), together with the seven
// completed Route 23 badge checks. Backtracking south intentionally makes the
// fact false again because the cave puzzle itself has been reset and must be
// traversed again.
func victoryRoadClearedForProgress(mem *state.Mem, facts state.StoryFacts) bool {
	if facts.LeagueChallengeStarted || facts.LeagueChampionDefeated || facts.MainStoryComplete {
		return true
	}
	if state.VictoryRoadCleared(mem) {
		return true
	}
	if !facts.Route23BadgeChecksComplete {
		return false
	}
	switch mem.U8(sym.CurMap) {
	case indigoPlateauMap, indigoPlateauLobbyMap:
		return true
	case profileRoute23Map:
		return state.Route23NorthOfVictoryRoad(int(mem.U8(sym.XCoord)), int(mem.U8(sym.YCoord)))
	default:
		return false
	}
}
