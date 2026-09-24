package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func victoryRoadStageState(m *emu.Emu, policy MovePolicy) (state.Mem, state.StoryFacts, error) {
	if policy == nil {
		return state.Mem{}, state.StoryFacts{}, fmt.Errorf("skill: Victory Road stage: nil move policy")
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	facts := state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem))
	if facts.LeagueChallengeStarted || facts.LeagueChampionDefeated || facts.MainStoryComplete {
		return mem, facts, nil
	}
	if state.DecodeProgress(&mem).BadgeCount != 8 {
		return state.Mem{}, state.StoryFacts{}, gameruntime.NewProgressionPrerequisiteMissing("earth_badge")
	}
	return mem, facts, nil
}

// victoryRoadClearBoundary mirrors the profile's semantic clear fact for skill
// preconditions. Route 23 resets the live boulder events, so the final switch
// proves completion inside the cave while a position north of the cave/at
// Indigo proves a successful exit afterward. If the run backtracks south, the
// fact intentionally becomes false because the puzzle has reset.
func victoryRoadClearBoundary(mem *state.Mem, facts state.StoryFacts) bool {
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
	case route23Map:
		return int(mem.U8(sym.YCoord)) <= route23NorthCaveY
	default:
		return false
	}
}

// VictoryRoadResolveRival owns only the final Route 22 rival transaction. The
// completion event is already projected as route_22_rival_resolved, so a
// checkpoint after the battle resumes at the Route 23 stage without replay.
func VictoryRoadResolveRival(m *emu.Emu, romData []byte, policy MovePolicy) error {
	_, facts, err := victoryRoadStageState(m, policy)
	if err != nil {
		return err
	}
	if facts.Route22RivalResolved || facts.LeagueChallengeStarted {
		return nil
	}
	if err := resolveRoute22LeagueRival(m, romData, policy); err != nil {
		return fmt.Errorf("skill: VictoryRoadResolveRival: %w", err)
	}
	if !currentStoryFacts(m).Route22RivalResolved {
		return fmt.Errorf("skill: VictoryRoadResolveRival: rival completion event is still false")
	}
	return nil
}

// victoryRoadReachEntryFromCurrentState re-establishes the cave-entry boundary
// for a resumed stage. The normal path starts on Route 23 immediately after the
// rival stage; the broader routing exists so a checkpoint that wandered away
// can still recover without replaying an unrelated canonical input sequence.
func victoryRoadReachEntryFromCurrentState(m *emu.Emu, romData []byte, policy MovePolicy) error {
	cur := m.Peek8(sym.CurMap)
	if inVictoryRoad(cur) {
		return nil
	}
	if cur == indigoPlateauMap || cur == indigoPlateauLobbyMap {
		return nil
	}
	if cur != route23Map {
		if _, err := TravelFlee(m, romData, route23SouthEntry, policy, victoryRoadTravelBattles); err != nil {
			return fmt.Errorf("reach Route 23 south entry: %w", err)
		}
	}
	if got := m.Peek8(sym.CurMap); got != route23Map {
		return fmt.Errorf("expected Route 23 before badge-check traversal, observed map %#02x", got)
	}
	for _, barrierY := range route23SurfBarrierRows {
		if err := crossRoute23SurfBandNorth(m, romData, policy, barrierY); err != nil {
			return err
		}
	}
	if _, err := TravelFlee(m, romData, victoryRoad1FEntry, policy, victoryRoadTravelBattles); err != nil {
		return fmt.Errorf("pass final Route 23 badge checks and enter Victory Road: %w", err)
	}
	return nil
}

// VictoryRoadReachCave owns Route 23: it requires the final rival fact, repairs
// Surf, crosses the three water bands and all seven badge checks, and ends at
// the Victory Road 1F entry. Its semantic postcondition is the existing
// route_23_badge_checks fact (7/7).
func VictoryRoadReachCave(m *emu.Emu, romData []byte, policy MovePolicy) error {
	_, facts, err := victoryRoadStageState(m, policy)
	if err != nil {
		return err
	}
	if facts.LeagueChallengeStarted || facts.Route23BadgeChecksComplete {
		return nil
	}
	if !facts.Route22RivalResolved {
		return gameruntime.NewProgressionPrerequisiteMissing("route_22_rival_resolved")
	}
	if err := RepairFieldCapabilities(m, romData, policy, []FieldMove{FieldSurf}); err != nil {
		return fmt.Errorf("skill: VictoryRoadReachCave: prepare Surf: %w", err)
	}
	if err := victoryRoadReachEntryFromCurrentState(m, romData, policy); err != nil {
		return fmt.Errorf("skill: VictoryRoadReachCave: %w", err)
	}
	facts = currentStoryFacts(m)
	if !facts.Route23BadgeChecksComplete || facts.Route23BadgeChecksPassed != 7 {
		return fmt.Errorf("skill: VictoryRoadReachCave: Route 23 badge checks = %d/7 after cave entry", facts.Route23BadgeChecksPassed)
	}
	return nil
}

// VictoryRoadClearCave owns the live Strength puzzle chain only. On the normal
// path it starts at 1F from VictoryRoadReachCave; on a resumed/backtracked run
// it can re-establish the entry first. Completion is the final 2F east switch
// or a verified post-cave position before the Route 23 reset can erase it.
func VictoryRoadClearCave(m *emu.Emu, romData []byte, policy MovePolicy) error {
	mem, facts, err := victoryRoadStageState(m, policy)
	if err != nil {
		return err
	}
	if victoryRoadClearBoundary(&mem, facts) {
		return nil
	}
	if !facts.Route23BadgeChecksComplete {
		return gameruntime.NewProgressionPrerequisiteMissing("route_23_badge_checks")
	}
	if err := RepairFieldCapabilities(m, romData, policy, []FieldMove{FieldSurf, FieldStrength}); err != nil {
		return fmt.Errorf("skill: VictoryRoadClearCave: prepare Surf + Strength: %w", err)
	}
	if !inVictoryRoad(m.Peek8(sym.CurMap)) {
		if err := victoryRoadReachEntryFromCurrentState(m, romData, policy); err != nil {
			return fmt.Errorf("skill: VictoryRoadClearCave: restore cave entry: %w", err)
		}
	}
	if !inVictoryRoad(m.Peek8(sym.CurMap)) {
		return fmt.Errorf("skill: VictoryRoadClearCave: expected a Victory Road floor, observed map %#02x", m.Peek8(sym.CurMap))
	}
	if err := clearVictoryRoad(m, romData, policy); err != nil {
		return fmt.Errorf("skill: VictoryRoadClearCave: %w", err)
	}
	state.Snapshot(m, &mem)
	facts = state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem))
	if !victoryRoadClearBoundary(&mem, facts) {
		return fmt.Errorf("skill: VictoryRoadClearCave: final cave-clear boundary is still false")
	}
	return nil
}

// VictoryRoadPrepareIndigo owns only the cave-exit -> Indigo lobby checkpoint
// and Center recovery. Once LeagueChallengeStarted is durable, readiness stays
// satisfied even though battles naturally damage the party afterward.
func VictoryRoadPrepareIndigo(m *emu.Emu, romData []byte, policy MovePolicy) error {
	mem, facts, err := victoryRoadStageState(m, policy)
	if err != nil {
		return err
	}
	if facts.LeagueChallengeStarted || facts.LeagueChampionDefeated || facts.MainStoryComplete {
		return nil
	}
	if mem.U8(sym.CurMap) == indigoPlateauLobbyMap && allPartyCenterRecovered(&mem) {
		return nil
	}
	if !victoryRoadClearBoundary(&mem, facts) {
		return gameruntime.NewProgressionPrerequisiteMissing("victory_road_cleared")
	}
	if err := prepareIndigoLobby(m, romData, policy); err != nil {
		return fmt.Errorf("skill: VictoryRoadPrepareIndigo: %w", err)
	}
	state.Snapshot(m, &mem)
	if mem.U8(sym.CurMap) != indigoPlateauLobbyMap || !allPartyCenterRecovered(&mem) {
		return fmt.Errorf("skill: VictoryRoadPrepareIndigo: lobby recovery postcondition failed")
	}
	return nil
}
