package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

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
		if err := settleLeagueRoomEntry(m, loreleiRoomMap, false); err != nil {
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
	for hop := 0; hop < 8; hop++ {
		if m.Peek8(sym.CurMap) == targetMap {
			return nil
		}
		facts := currentLeagueFacts(m)
		switch m.Peek8(sym.CurMap) {
		case indigoPlateauMap:
			if err := recoverLeagueBlackout(m, romData, policy); err != nil {
				return err
			}
		case indigoPlateauLobbyMap:
			if err := prepareLeagueChallenge(m, romData, policy); err != nil {
				return err
			}
		case loreleiRoomMap:
			if !facts.LeagueLoreleiDefeated {
				return fmt.Errorf("Lorelei must be defeated before advancing toward map %#02x", targetMap)
			}
			if err := enterLeagueRoom(m, romData, policy, loreleiExitStand, brunoRoomMap, false); err != nil {
				return err
			}
		case brunoRoomMap:
			if !facts.LeagueBrunoDefeated {
				return fmt.Errorf("Bruno must be defeated before advancing toward map %#02x", targetMap)
			}
			if err := enterLeagueRoom(m, romData, policy, brunoExitStand, agathaRoomMap, false); err != nil {
				return err
			}
		case agathaRoomMap:
			if !facts.LeagueAgathaDefeated {
				return fmt.Errorf("Agatha must be defeated before advancing toward map %#02x", targetMap)
			}
			if err := enterLeagueRoom(m, romData, policy, agathaExitStand, lanceRoomMap, false); err != nil {
				return err
			}
		case lanceRoomMap:
			if !facts.LeagueLanceDefeated {
				return fmt.Errorf("Lance must be defeated before advancing toward map %#02x", targetMap)
			}
			if err := enterLeagueRoom(m, romData, policy, lanceExitStand, championsRoomMap, true); err != nil {
				return err
			}
		default:
			return fmt.Errorf("cannot advance League stage from map %#02x toward %#02x", m.Peek8(sym.CurMap), targetMap)
		}
	}
	return fmt.Errorf("exceeded bounded League room transitions while reaching map %#02x", targetMap)
}

func LeagueDefeatLorelei(m *emu.Emu, romData []byte, policy MovePolicy) error {
	facts := currentLeagueFacts(m)
	if facts.MainStoryComplete || facts.LeagueLoreleiDefeated {
		return nil
	}
	if !facts.LeagueChallengeStarted {
		if err := LeagueStartChallenge(m, romData, policy); err != nil {
			return err
		}
	}
	if err := leagueReachRoom(m, romData, policy, loreleiRoomMap); err != nil {
		return fmt.Errorf("skill: LeagueDefeatLorelei: %w", err)
	}
	return fightLeagueMember(m, romData, policy, "Lorelei", 5, 2, func(f state.StoryFacts) bool { return f.LeagueLoreleiDefeated })
}

func LeagueDefeatBruno(m *emu.Emu, romData []byte, policy MovePolicy) error {
	facts := currentLeagueFacts(m)
	if facts.MainStoryComplete || facts.LeagueBrunoDefeated {
		return nil
	}
	if !facts.LeagueLoreleiDefeated {
		return fmt.Errorf("skill: LeagueDefeatBruno: Lorelei is not defeated")
	}
	if err := leagueReachRoom(m, romData, policy, brunoRoomMap); err != nil {
		return fmt.Errorf("skill: LeagueDefeatBruno: %w", err)
	}
	return fightLeagueMember(m, romData, policy, "Bruno", 5, 2, func(f state.StoryFacts) bool { return f.LeagueBrunoDefeated })
}

func LeagueDefeatAgatha(m *emu.Emu, romData []byte, policy MovePolicy) error {
	facts := currentLeagueFacts(m)
	if facts.MainStoryComplete || facts.LeagueAgathaDefeated {
		return nil
	}
	if !facts.LeagueBrunoDefeated {
		return fmt.Errorf("skill: LeagueDefeatAgatha: Bruno is not defeated")
	}
	if err := leagueReachRoom(m, romData, policy, agathaRoomMap); err != nil {
		return fmt.Errorf("skill: LeagueDefeatAgatha: %w", err)
	}
	return fightLeagueMember(m, romData, policy, "Agatha", 5, 2, func(f state.StoryFacts) bool { return f.LeagueAgathaDefeated })
}

func LeagueDefeatLance(m *emu.Emu, romData []byte, policy MovePolicy) error {
	facts := currentLeagueFacts(m)
	if facts.MainStoryComplete || facts.LeagueLanceDefeated {
		return nil
	}
	if !facts.LeagueAgathaDefeated {
		return fmt.Errorf("skill: LeagueDefeatLance: Agatha is not defeated")
	}
	if err := leagueReachRoom(m, romData, policy, lanceRoomMap); err != nil {
		return fmt.Errorf("skill: LeagueDefeatLance: %w", err)
	}
	return fightLeagueMember(m, romData, policy, "Lance", 6, 1, func(f state.StoryFacts) bool { return f.LeagueLanceDefeated })
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
			return fmt.Errorf("Champion battle: %w", err)
		}
		if outcome != state.ResultWon {
			return fmt.Errorf("%w against Champion", ErrTrainerBlackedOut)
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
	facts := currentLeagueFacts(m)
	if facts.MainStoryComplete || facts.LeagueChampionDefeated {
		return nil
	}
	if !facts.LeagueLanceDefeated {
		return fmt.Errorf("skill: LeagueDefeatChampion: Lance is not defeated")
	}
	if err := leagueReachRoom(m, romData, policy, championsRoomMap); err != nil {
		return fmt.Errorf("skill: LeagueDefeatChampion: %w", err)
	}
	if err := fightChampionStage(m, policy); err != nil {
		return fmt.Errorf("skill: LeagueDefeatChampion: %w", err)
	}
	return nil
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
		return fmt.Errorf("skill: LeagueFinishHallOfFame: Champion is not defeated")
	}
	if err := finishHallOfFame(m); err != nil {
		return fmt.Errorf("skill: LeagueFinishHallOfFame: %w", err)
	}
	return nil
}
