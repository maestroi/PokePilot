package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func returnToLeagueCheckpoint(m *emu.Emu, romData []byte, policy MovePolicy) error {
	currentMap := m.Peek8(sym.CurMap)
	switch currentMap {
	case indigoPlateauMap, indigoPlateauLobbyMap:
		return nil
	}
	if _, ok := leagueStageForRoom(currentMap); ok {
		return nil
	}
	nurse, err := indigoLobbyNurseDestination(romData)
	if err != nil {
		return fmt.Errorf("resolve Indigo checkpoint: %w", err)
	}
	if _, err := TravelFlee(m, romData, nurse, policy, leagueTravelBattles); err != nil {
		return fmt.Errorf("return to Indigo checkpoint: %w", err)
	}
	if got := m.Peek8(sym.CurMap); got != indigoPlateauLobbyMap {
		return fmt.Errorf("return to Indigo checkpoint ended on map %#02x, want %#02x", got, indigoPlateauLobbyMap)
	}
	return nil
}

// LeagueStartChallenge owns only the prepared-lobby -> Lorelei-room boundary.
// A blackout may place Red on the Indigo exterior; in that case this stage
// returns to the lobby, heals if needed, and recommits to the League. The
// durable postcondition is LeagueChallengeStarted.
func LeagueStartChallenge(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: LeagueStartChallenge: nil policy")
	}
	facts := currentLeagueFacts(m)
	if facts.MainStoryComplete || facts.LeagueChampionDefeated || facts.LeagueChallengeStarted {
		return nil
	}
	if err := returnToLeagueCheckpoint(m, romData, policy); err != nil {
		return fmt.Errorf("skill: LeagueStartChallenge: %w", err)
	}

	if m.Peek8(sym.CurMap) == indigoPlateauMap {
		if err := recoverLeagueBlackout(m, romData, policy); err != nil {
			return fmt.Errorf("skill: LeagueStartChallenge: %w", err)
		}
	}
	if m.Peek8(sym.CurMap) == indigoPlateauLobbyMap {
		if err := prepareLeagueChallenge(m, romData, policy); err != nil {
			return fmt.Errorf("skill: LeagueStartChallenge: %w", err)
		}
	}
	if m.Peek8(sym.CurMap) == loreleiRoomMap && !currentLeagueFacts(m).LeagueChallengeStarted {
		if err := settleLeagueRoomEntry(m, romData, loreleiRoomMap, false); err != nil {
			return fmt.Errorf("skill: LeagueStartChallenge: settle Lorelei entry: %w", err)
		}
	}
	if !currentLeagueFacts(m).LeagueChallengeStarted {
		return fmt.Errorf("skill: LeagueStartChallenge: League-started fact is still false on map %#02x", m.Peek8(sym.CurMap))
	}
	return nil
}

// leagueReachRoom advances only through already-completed League rooms. It
// never defeats a trainer on behalf of a later stage. That keeps each objective
// transaction bounded while still making resumed checkpoints and blackout
// retries recoverable from the nearest semantic boundary.
func leagueReachRoom(m *emu.Emu, romData []byte, policy MovePolicy, targetMap uint8) error {
	if err := returnToLeagueCheckpoint(m, romData, policy); err != nil {
		return err
	}
	for hop := 0; hop < 8; hop++ {
		currentMap := m.Peek8(sym.CurMap)
		if currentMap == targetMap {
			return nil
		}
		switch currentMap {
		case indigoPlateauMap:
			if err := recoverLeagueBlackout(m, romData, policy); err != nil {
				return err
			}
			continue
		case indigoPlateauLobbyMap:
			if err := prepareLeagueChallenge(m, romData, policy); err != nil {
				return err
			}
			continue
		}

		stage, ok := leagueStageForRoom(currentMap)
		if !ok {
			return fmt.Errorf("cannot advance League stage from map %#02x toward %#02x", currentMap, targetMap)
		}
		if !stage.Done(currentLeagueFacts(m)) {
			return gameruntime.NewProgressionPrerequisiteMissing(stage.ID)
		}
		if stage.Exit == nil {
			return fmt.Errorf("cannot advance past terminal League stage %s toward map %#02x", stage.Name, targetMap)
		}
		if err := enterLeagueRoom(
			m,
			romData,
			policy,
			stage.Exit.Stand,
			stage.Exit.NextRoom,
			stage.Exit.WaitForBattle,
		); err != nil {
			return err
		}
	}
	return fmt.Errorf("exceeded bounded League room transitions while reaching map %#02x", targetMap)
}

func LeagueDefeatLorelei(m *emu.Emu, romData []byte, policy MovePolicy) error {
	return runLeagueStageByID(m, romData, policy, leagueProgressLoreleiDefeated)
}

func LeagueDefeatBruno(m *emu.Emu, romData []byte, policy MovePolicy) error {
	return runLeagueStageByID(m, romData, policy, leagueProgressBrunoDefeated)
}

func LeagueDefeatAgatha(m *emu.Emu, romData []byte, policy MovePolicy) error {
	return runLeagueStageByID(m, romData, policy, leagueProgressAgathaDefeated)
}

func LeagueDefeatLance(m *emu.Emu, romData []byte, policy MovePolicy) error {
	return runLeagueStageByID(m, romData, policy, leagueProgressLanceDefeated)
}

func fightChampionStage(m *emu.Emu, policy MovePolicy) error {
	facts := currentLeagueFacts(m)
	if facts.MainStoryComplete || facts.LeagueChampionDefeated {
		return nil
	}
	if m.Peek8(sym.CurMap) != championsRoomMap {
		return fmt.Errorf("Champion stage on map %#02x, want %#02x", m.Peek8(sym.CurMap), championsRoomMap)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) == nil {
		mem = advanceUntil(m, leagueRoomSettleBudget, func(mm *state.Mem) bool {
			facts := leagueFacts(mm)
			return state.DecodeBattle(mm) != nil || facts.LeagueChampionDefeated || facts.MainStoryComplete
		})
	}
	facts = leagueFacts(&mem)
	if !facts.LeagueChampionDefeated && !facts.MainStoryComplete {
		if state.DecodeBattle(&mem) == nil {
			return fmt.Errorf("Champion room did not enter battle")
		}
		outcome, err := Battle(m, policy)
		if err != nil {
			// A win hands control to Oak's scene and the Hall of Fame, not
			// back to the player, so Battle's post-battle controllable
			// settle times out. The committed victory is the positive proof.
			if won := currentLeagueFacts(m); !won.LeagueChampionDefeated && !won.MainStoryComplete {
				return fmt.Errorf("Champion battle: %w", err)
			}
		} else if err := RequireTrainerBattleWin("league:champion", outcome); err != nil {
			return fmt.Errorf("Champion battle: %w", err)
		}
	}
	mem = advanceUntil(m, leagueBattleSettleBudget, func(mm *state.Mem) bool {
		facts := leagueFacts(mm)
		return facts.LeagueChampionDefeated || facts.MainStoryComplete
	})
	facts = leagueFacts(&mem)
	if !facts.LeagueChampionDefeated && !facts.MainStoryComplete {
		return fmt.Errorf("Champion win did not commit its victory event")
	}
	return nil
}

func LeagueDefeatChampion(m *emu.Emu, romData []byte, policy MovePolicy) error {
	return runLeagueStageByID(m, romData, policy, leagueProgressChampionDefeated)
}

// LeagueFinishHallOfFame owns only the post-Champion ending scripts. The
// durable wElite4Flags completion bit, not the transient Champion event, is the
// positive boundary proving that the Hall of Fame was actually recorded.
func LeagueFinishHallOfFame(m *emu.Emu) error {
	facts := currentLeagueFacts(m)
	if facts.MainStoryComplete {
		return nil
	}
	if !facts.LeagueChampionDefeated {
		return gameruntime.NewProgressionPrerequisiteMissing("league_champion_defeated")
	}
	if err := finishHallOfFame(m); err != nil {
		return fmt.Errorf("skill: LeagueFinishHallOfFame: %w", err)
	}
	return nil
}
