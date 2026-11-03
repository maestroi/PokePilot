package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// LeagueRematch replays the whole League gauntlet on a save that already
// reached the Hall of Fame. Red keeps the League open after the ending (the
// lobby and HallOfFameResetEventsAndSaveScript both erase the Indigo events),
// and every member pays prize money, so it is the game's only repeatable
// income once every other trainer is beaten.
//
// The rematch reuses the campaign's stage controllers with attempt-scoped
// facts (state.DecodeLeagueAttemptFacts). A loss surfaces the same typed
// required-battle error as the campaign; the blackout itself is the game's
// own consequence and the caller's postcondition decides success.
func LeagueRematch(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: LeagueRematch: nil policy")
	}
	if !leagueMainStoryComplete(m) {
		return gameruntime.NewProgressionPrerequisiteMissing(gen1.ProgressMainStoryComplete)
	}
	leagueRematches.Store(m, struct{}{})
	defer leagueRematches.Delete(m)

	if err := reachIndigoForRematch(m, romData, policy); err != nil {
		return fmt.Errorf("skill: LeagueRematch: %w", err)
	}
	if err := returnToLeagueCheckpoint(m, romData, policy); err != nil {
		return fmt.Errorf("skill: LeagueRematch: %w", err)
	}
	if err := EliteFourProgression(m, romData, policy); err != nil {
		return fmt.Errorf("skill: LeagueRematch: %w", err)
	}
	return nil
}

// finishRematchHallOfFame plays a repeat ending through its own save. The
// first campaign keys on the durable completion bit, which a rematch already
// has, and ensureNavigableOverworld soft-resets on the Hall of Fame map once
// that bit is set — before the ending's SaveGameData that would throw away
// the attempt's prize money. HallOfFameResetEventsAndSaveScript clears the
// Champion event, saves, waits for a button and then jumps to Init, so the
// post-Init title boundary with the Champion event cleared is the positive
// proof the prize money was saved.
func finishRematchHallOfFame(m *emu.Emu) error {
	var mem state.Mem
	saved := func() bool {
		state.Snapshot(m, &mem)
		return !state.HasEvent(&mem, state.EventBeatChampionRival) && postInitTitleBoundary(&mem)
	}
	for spent := 0; !saved(); spent += leagueEndingPressEvery {
		if spent >= leagueEndingBudget {
			return fmt.Errorf("skill: LeagueRematch: Hall of Fame did not save and reset within %d frames; map=%#02x", leagueEndingBudget, mem.U8(sym.CurMap))
		}
		m.Tap(emu.A, 3, 7)
		m.StepFrames(leagueEndingPressEvery - 10)
	}
	if err := ensureNavigableOverworld(m); err != nil {
		return fmt.Errorf("skill: LeagueRematch: leave Hall of Fame ending: %w", err)
	}
	if got := m.Peek8(sym.CurMap); got == hallOfFameMap {
		return fmt.Errorf("skill: LeagueRematch: still on Hall of Fame after ending exit")
	}
	return nil
}

// reachIndigoForRematch brings a post-game save back to the Indigo lobby. A
// Fly-capable party travels normally. Otherwise the only way is Victory Road
// again, and its boulder switches reset (Route 23 and the 2F map script clear
// them), so it reuses the campaign's cave controllers rather than generic
// routing: run-d6dokr184ky81 walked from Seafoam into Victory Road 3F and
// found no route past the reset boulders.
func reachIndigoForRematch(m *emu.Emu, romData []byte, policy MovePolicy) error {
	cur := m.Peek8(sym.CurMap)
	if _, inLeague := leagueStageForRoom(cur); inLeague || cur == indigoPlateauMap || cur == indigoPlateauLobbyMap {
		return nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	northOfCave := cur == route23Map && state.Route23NorthOfVictoryRoad(int(mem.U8(sym.XCoord)), int(mem.U8(sym.YCoord)))
	if northOfCave || FieldCapabilityFor(&mem, FieldFly).Usable {
		nurse, err := indigoLobbyNurseDestination(romData)
		if err != nil {
			return err
		}
		if _, err := TravelFlee(m, romData, nurse, policy, victoryRoadTravelBattles); err != nil {
			return fmt.Errorf("reach Indigo lobby: %w", err)
		}
		return nil
	}
	var missing []gameruntime.CapabilityID
	if !FieldCapabilityFor(&mem, FieldSurf).Usable {
		missing = append(missing, "surf")
	}
	if !FieldCapabilityFor(&mem, FieldStrength).Usable {
		missing = append(missing, "strength")
	}
	if len(missing) != 0 {
		return gameruntime.NewFieldCapabilityPrerequisiteMissing(missing...)
	}
	if err := victoryRoadReachEntryFromCurrentState(m, romData, policy); err != nil {
		return fmt.Errorf("reach Victory Road: %w", err)
	}
	if err := clearVictoryRoad(m, romData, policy); err != nil {
		return err
	}
	return prepareIndigoLobby(m, romData, policy)
}
