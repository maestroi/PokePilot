package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	loreleiRoomMap   uint8 = 0xF5
	brunoRoomMap     uint8 = 0xF6
	agathaRoomMap    uint8 = 0xF7
	lanceRoomMap     uint8 = 0x71
	championsRoomMap uint8 = 0x78
	hallOfFameMap    uint8 = 0x76

	leagueTravelBattles      = 8
	leagueWarpBudget         = 600
	leagueRoomSettleBudget   = 12000
	leagueBattleSettleBudget = 12000
	leagueEndingBudget       = 120000
)

var (
	leagueLobbyExitStand = Destination{Map: indigoPlateauLobbyMap, X: 8, Y: 1}
	loreleiExitStand     = Destination{Map: loreleiRoomMap, X: 4, Y: 1}
	brunoExitStand       = Destination{Map: brunoRoomMap, X: 4, Y: 1}
	agathaExitStand      = Destination{Map: agathaRoomMap, X: 4, Y: 1}
	lanceExitStand       = Destination{Map: lanceRoomMap, X: 5, Y: 1}
)

type leagueFact func(state.StoryFacts) bool

func leagueFacts(mem *state.Mem) state.StoryFacts {
	return state.DecodeStoryFacts(mem, state.DecodeInventory(mem))
}

func currentLeagueFacts(m *emu.Emu) state.StoryFacts {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return leagueFacts(&mem)
}

func leagueMainStoryComplete(m *emu.Emu) bool {
	return currentLeagueFacts(m).MainStoryComplete
}

// enterLeagueRoom crosses one north-facing League warp without assuming how
// long the destination's scripted entrance takes. Lorelei, Bruno, and Agatha
// each force-walk Red six tiles into the room; Lance performs a much longer
// hallway walk; the Champion room drives its own rival entrance and dialogue.
// The helper releases UP the frame the map changes, then waits on decoded game
// state rather than a fixed input macro.
func enterLeagueRoom(m *emu.Emu, romData []byte, policy MovePolicy, stand Destination, targetMap uint8, waitForBattle bool) error {
	if got := m.Peek8(sym.CurMap); got == targetMap {
		return settleLeagueRoomEntry(m, targetMap, waitForBattle)
	} else if got != stand.Map {
		return fmt.Errorf("skill: EliteFourProgression: north warp expected map %#02x, observed %#02x", stand.Map, got)
	}

	if _, err := TravelFlee(m, romData, stand, policy, leagueTravelBattles); err != nil {
		return fmt.Errorf("skill: EliteFourProgression: reach north exit on map %#02x: %w", stand.Map, err)
	}
	if got := m.Peek8(sym.CurMap); got != stand.Map {
		return fmt.Errorf("skill: EliteFourProgression: approach to north exit left map %#02x for %#02x", stand.Map, got)
	}

	m.Press(emu.Up)
	crossed := false
	for spent := 0; spent < leagueWarpBudget; spent++ {
		if m.Peek8(sym.CurMap) != stand.Map {
			crossed = true
			break
		}
		m.StepFrame()
	}
	m.Release(emu.Up)
	if !crossed {
		x, y := playerXY(m)
		return fmt.Errorf("skill: EliteFourProgression: north exit from map %#02x did not cross within %d frames at (%d,%d)", stand.Map, leagueWarpBudget, x, y)
	}
	if got := m.Peek8(sym.CurMap); got != targetMap {
		return fmt.Errorf("skill: EliteFourProgression: north exit from map %#02x arrived on %#02x, want %#02x", stand.Map, got, targetMap)
	}
	return settleLeagueRoomEntry(m, targetMap, waitForBattle)
}

func settleLeagueRoomEntry(m *emu.Emu, targetMap uint8, waitForBattle bool) error {
	mem := advanceUntil(m, leagueRoomSettleBudget, func(mm *state.Mem) bool {
		if mm.U8(sym.CurMap) != targetMap {
			return false
		}
		if leagueFacts(mm).MainStoryComplete {
			return true
		}
		if waitForBattle {
			return state.DecodeBattle(mm) != nil || leagueFacts(mm).LeagueChampionDefeated
		}
		return state.Controllable(mm)
	})
	if mem.U8(sym.CurMap) != targetMap {
		return fmt.Errorf("skill: EliteFourProgression: room entry expected map %#02x, observed %#02x", targetMap, mem.U8(sym.CurMap))
	}
	if leagueFacts(&mem).MainStoryComplete {
		return nil
	}
	if waitForBattle {
		if state.DecodeBattle(&mem) == nil && !leagueFacts(&mem).LeagueChampionDefeated {
			return fmt.Errorf("skill: EliteFourProgression: Champion battle did not start within %d frames", leagueRoomSettleBudget)
		}
		return nil
	}
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: EliteFourProgression: map %#02x did not settle controllable within %d frames", targetMap, leagueRoomSettleBudget)
	}
	return nil
}

func fightLeagueMember(m *emu.Emu, romData []byte, policy MovePolicy, name string, homeX, homeY uint8, done leagueFact) error {
	if done(currentLeagueFacts(m)) {
		return nil
	}
	if err := ChallengeTrainer(m, romData, homeX, homeY, policy); err != nil {
		return fmt.Errorf("skill: EliteFourProgression: %s: %w", name, err)
	}
	mem := advanceUntil(m, leagueBattleSettleBudget, func(mm *state.Mem) bool {
		return done(leagueFacts(mm)) && state.Controllable(mm)
	})
	facts := leagueFacts(&mem)
	if !done(facts) {
		return fmt.Errorf("skill: EliteFourProgression: %s battle won but its room-completion fact was not committed", name)
	}
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: EliteFourProgression: %s post-battle script did not return control", name)
	}
	return nil
}

func prepareLeagueChallenge(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if got := m.Peek8(sym.CurMap); got != indigoPlateauLobbyMap {
		return fmt.Errorf("skill: EliteFourProgression: prepare League on map %#02x, want Indigo lobby %#02x", got, indigoPlateauLobbyMap)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !allPartyCenterRecovered(&mem) {
		if _, err := TravelFlee(m, romData, indigoLobbyNurse, policy, leagueTravelBattles); err != nil {
			return fmt.Errorf("skill: EliteFourProgression: reach Indigo nurse: %w", err)
		}
		if err := Heal(m); err != nil {
			return fmt.Errorf("skill: EliteFourProgression: heal before committing to the League: %w", err)
		}
		state.Snapshot(m, &mem)
		if !allPartyCenterRecovered(&mem) {
			return fmt.Errorf("skill: EliteFourProgression: Indigo heal did not fully restore HP/status/PP")
		}
	}
	if err := enterLeagueRoom(m, romData, policy, leagueLobbyExitStand, loreleiRoomMap, false); err != nil {
		return err
	}
	if !currentLeagueFacts(m).LeagueChallengeStarted {
		return fmt.Errorf("skill: EliteFourProgression: Lorelei room entry did not set the League-started fact")
	}
	return nil
}

func recoverLeagueBlackout(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if got := m.Peek8(sym.CurMap); got != indigoPlateauMap {
		return fmt.Errorf("skill: EliteFourProgression: League blackout recovery started on map %#02x, want Indigo Plateau %#02x", got, indigoPlateauMap)
	}
	if _, err := TravelFlee(m, romData, indigoLobbyNurse, policy, leagueTravelBattles); err != nil {
		return fmt.Errorf("skill: EliteFourProgression: return to Indigo lobby after blackout: %w", err)
	}
	if got := m.Peek8(sym.CurMap); got != indigoPlateauLobbyMap {
		return fmt.Errorf("skill: EliteFourProgression: blackout recovery arrived on map %#02x, want lobby %#02x", got, indigoPlateauLobbyMap)
	}
	return nil
}

func fightChampion(m *emu.Emu, policy MovePolicy) error {
	facts := currentLeagueFacts(m)
	if facts.MainStoryComplete {
		return nil
	}
	if !facts.LeagueChampionDefeated {
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
				return fmt.Errorf("skill: EliteFourProgression: Champion room did not enter battle")
			}
			outcome, err := Battle(m, policy)
			if err != nil {
				return fmt.Errorf("skill: EliteFourProgression: Champion battle: %w", err)
			}
			if outcome != state.ResultWon {
				return fmt.Errorf("skill: EliteFourProgression: %w against Champion", ErrTrainerBlackedOut)
			}
		}
		mem = advanceUntil(m, leagueBattleSettleBudget, func(mm *state.Mem) bool {
			facts := leagueFacts(mm)
			return facts.LeagueChampionDefeated || facts.MainStoryComplete
		})
		facts = leagueFacts(&mem)
		if !facts.LeagueChampionDefeated && !facts.MainStoryComplete {
			return fmt.Errorf("skill: EliteFourProgression: Champion win did not commit its victory event")
		}
	}
	return finishHallOfFame(m)
}

// finishHallOfFame advances the Champion/Oak/Hall-of-Fame scripts until the
// ending records its durable completion bit. The Champion event alone is not
// enough: HallOfFameResetEventsAndSaveScript later clears the entire Indigo
// event range. The durable wElite4Flags completion bit is therefore the only
// positive fact that means the main game actually reached the Hall of Fame.
func finishHallOfFame(m *emu.Emu) error {
	if leagueMainStoryComplete(m) {
		return nil
	}
	mem := advanceUntil(m, leagueEndingBudget, func(mm *state.Mem) bool {
		return leagueFacts(mm).MainStoryComplete
	})
	facts := leagueFacts(&mem)
	if !facts.MainStoryComplete {
		return fmt.Errorf("skill: EliteFourProgression: Hall of Fame did not commit main-story completion within %d frames; map=%#02x", leagueEndingBudget, mem.U8(sym.CurMap))
	}
	if mem.U8(sym.CurMap) != hallOfFameMap {
		return fmt.Errorf("skill: EliteFourProgression: main-story completion appeared on map %#02x, want Hall of Fame %#02x", mem.U8(sym.CurMap), hallOfFameMap)
	}
	return nil
}

// EliteFourProgression owns the no-exit League gauntlet from a prepared Indigo
// Plateau lobby through Lorelei, Bruno, Agatha, Lance, the Champion, and the
// Hall of Fame. Every phase is resume-safe: current map plus durable event/RAM
// facts choose the next action, while battle losses preserve the typed
// ErrTrainerBlackedOut recovery path so the planner can train and retry.
func EliteFourProgression(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: EliteFourProgression: nil policy")
	}
	if leagueMainStoryComplete(m) {
		return nil
	}

	for phase := 0; phase < 16; phase++ {
		facts := currentLeagueFacts(m)
		if facts.MainStoryComplete {
			return nil
		}
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
			if err := fightLeagueMember(m, romData, policy, "Lorelei", 5, 2, func(f state.StoryFacts) bool { return f.LeagueLoreleiDefeated }); err != nil {
				return err
			}
			if err := enterLeagueRoom(m, romData, policy, loreleiExitStand, brunoRoomMap, false); err != nil {
				return err
			}

		case brunoRoomMap:
			if err := fightLeagueMember(m, romData, policy, "Bruno", 5, 2, func(f state.StoryFacts) bool { return f.LeagueBrunoDefeated }); err != nil {
				return err
			}
			if err := enterLeagueRoom(m, romData, policy, brunoExitStand, agathaRoomMap, false); err != nil {
				return err
			}

		case agathaRoomMap:
			if err := fightLeagueMember(m, romData, policy, "Agatha", 5, 2, func(f state.StoryFacts) bool { return f.LeagueAgathaDefeated }); err != nil {
				return err
			}
			if err := enterLeagueRoom(m, romData, policy, agathaExitStand, lanceRoomMap, false); err != nil {
				return err
			}

		case lanceRoomMap:
			if err := fightLeagueMember(m, romData, policy, "Lance", 6, 1, func(f state.StoryFacts) bool { return f.LeagueLanceDefeated }); err != nil {
				return err
			}
			if err := enterLeagueRoom(m, romData, policy, lanceExitStand, championsRoomMap, true); err != nil {
				return err
			}

		case championsRoomMap:
			return fightChampion(m, policy)

		case hallOfFameMap:
			return finishHallOfFame(m)

		default:
			return fmt.Errorf("skill: EliteFourProgression: unexpected map %#02x during League challenge (started=%v champion=%v)", m.Peek8(sym.CurMap), facts.LeagueChallengeStarted, facts.LeagueChampionDefeated)
		}
	}
	return fmt.Errorf("skill: EliteFourProgression: exceeded bounded room phase count")
}
