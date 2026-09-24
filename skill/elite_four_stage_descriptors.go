package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	leagueProgressChallengeStarted gameruntime.ProgressID = "league_challenge_started"
	leagueProgressLoreleiDefeated  gameruntime.ProgressID = "league_lorelei_defeated"
	leagueProgressBrunoDefeated    gameruntime.ProgressID = "league_bruno_defeated"
	leagueProgressAgathaDefeated   gameruntime.ProgressID = "league_agatha_defeated"
	leagueProgressLanceDefeated    gameruntime.ProgressID = "league_lance_defeated"
	leagueProgressChampionDefeated gameruntime.ProgressID = "league_champion_defeated"
)

type leagueStageFight func(*emu.Emu, []byte, MovePolicy, leagueStageDescriptor) error

type leagueStageExit struct {
	Stand               Destination
	NextRoom            uint8
	WaitForBattle       bool
	EncountersRemaining int
}

// leagueStageDescriptor is the single source of truth for the ordered League
// battle sequence. Generic stage execution consumes the semantic predecessor,
// completion fact, room, trainer interaction, and optional post-battle exit;
// Champion-specific battle/ending behavior stays behind an explicit fight hook.
type leagueStageDescriptor struct {
	Name            string
	Operation       string
	ID              gameruntime.ProgressID
	Predecessor     gameruntime.ProgressID
	RoomMap         uint8
	TrainerHomeX    uint8
	TrainerHomeY    uint8
	PredecessorDone leagueFact
	Done            leagueFact
	Fight           leagueStageFight
	Exit            *leagueStageExit
}

func standardLeagueStageFight(m *emu.Emu, romData []byte, policy MovePolicy, stage leagueStageDescriptor) error {
	return fightLeagueMember(
		m,
		romData,
		policy,
		stage.Name,
		stage.TrainerHomeX,
		stage.TrainerHomeY,
		stage.Done,
	)
}

func championLeagueStageFight(m *emu.Emu, _ []byte, policy MovePolicy, _ leagueStageDescriptor) error {
	return fightChampionStage(m, policy)
}

var leagueBattleStages = []leagueStageDescriptor{
	{
		Name:         "Lorelei",
		Operation:    "LeagueDefeatLorelei",
		ID:           leagueProgressLoreleiDefeated,
		Predecessor:  leagueProgressChallengeStarted,
		RoomMap:      loreleiRoomMap,
		TrainerHomeX: 5,
		TrainerHomeY: 2,
		PredecessorDone: func(f state.StoryFacts) bool {
			return f.LeagueChallengeStarted
		},
		Done: func(f state.StoryFacts) bool {
			return f.LeagueLoreleiDefeated
		},
		Fight: standardLeagueStageFight,
		Exit: &leagueStageExit{
			Stand:               loreleiExitStand,
			NextRoom:            brunoRoomMap,
			EncountersRemaining: 4,
		},
	},
	{
		Name:         "Bruno",
		Operation:    "LeagueDefeatBruno",
		ID:           leagueProgressBrunoDefeated,
		Predecessor:  leagueProgressLoreleiDefeated,
		RoomMap:      brunoRoomMap,
		TrainerHomeX: 5,
		TrainerHomeY: 2,
		PredecessorDone: func(f state.StoryFacts) bool {
			return f.LeagueLoreleiDefeated
		},
		Done: func(f state.StoryFacts) bool {
			return f.LeagueBrunoDefeated
		},
		Fight: standardLeagueStageFight,
		Exit: &leagueStageExit{
			Stand:               brunoExitStand,
			NextRoom:            agathaRoomMap,
			EncountersRemaining: 3,
		},
	},
	{
		Name:         "Agatha",
		Operation:    "LeagueDefeatAgatha",
		ID:           leagueProgressAgathaDefeated,
		Predecessor:  leagueProgressBrunoDefeated,
		RoomMap:      agathaRoomMap,
		TrainerHomeX: 5,
		TrainerHomeY: 2,
		PredecessorDone: func(f state.StoryFacts) bool {
			return f.LeagueBrunoDefeated
		},
		Done: func(f state.StoryFacts) bool {
			return f.LeagueAgathaDefeated
		},
		Fight: standardLeagueStageFight,
		Exit: &leagueStageExit{
			Stand:               agathaExitStand,
			NextRoom:            lanceRoomMap,
			EncountersRemaining: 2,
		},
	},
	{
		Name:         "Lance",
		Operation:    "LeagueDefeatLance",
		ID:           leagueProgressLanceDefeated,
		Predecessor:  leagueProgressAgathaDefeated,
		RoomMap:      lanceRoomMap,
		TrainerHomeX: 6,
		TrainerHomeY: 1,
		PredecessorDone: func(f state.StoryFacts) bool {
			return f.LeagueAgathaDefeated
		},
		Done: func(f state.StoryFacts) bool {
			return f.LeagueLanceDefeated
		},
		Fight: standardLeagueStageFight,
		Exit: &leagueStageExit{
			Stand:               lanceExitStand,
			NextRoom:            championsRoomMap,
			WaitForBattle:       true,
			EncountersRemaining: 1,
		},
	},
	{
		Name:        "Champion",
		Operation:   "LeagueDefeatChampion",
		ID:          leagueProgressChampionDefeated,
		Predecessor: leagueProgressLanceDefeated,
		RoomMap:     championsRoomMap,
		PredecessorDone: func(f state.StoryFacts) bool {
			return f.LeagueLanceDefeated
		},
		Done: func(f state.StoryFacts) bool {
			return f.LeagueChampionDefeated
		},
		Fight: championLeagueStageFight,
	},
}

func leagueStageForRoom(mapID uint8) (leagueStageDescriptor, bool) {
	for _, stage := range leagueBattleStages {
		if stage.RoomMap == mapID {
			return stage, true
		}
	}
	return leagueStageDescriptor{}, false
}

func leagueStageForID(id gameruntime.ProgressID) (leagueStageDescriptor, bool) {
	for _, stage := range leagueBattleStages {
		if stage.ID == id {
			return stage, true
		}
	}
	return leagueStageDescriptor{}, false
}

func earliestIncompleteLeagueStage(facts state.StoryFacts) (leagueStageDescriptor, bool) {
	for _, stage := range leagueBattleStages {
		if !stage.Done(facts) {
			return stage, true
		}
	}
	return leagueStageDescriptor{}, false
}

// runLeagueStage owns the shared bounded transaction for every League battle:
// semantic completion/predecessor checks, room recovery, member-specific fight,
// and positive completion verification. The descriptor's Fight hook is the only
// stage-specific mechanics seam.
func runLeagueStageByID(m *emu.Emu, romData []byte, policy MovePolicy, id gameruntime.ProgressID) error {
	stage, ok := leagueStageForID(id)
	if !ok {
		return fmt.Errorf("skill: League stage %q is not declared", id)
	}
	return runLeagueStage(m, romData, policy, stage)
}

func runLeagueStage(m *emu.Emu, romData []byte, policy MovePolicy, stage leagueStageDescriptor) error {
	if policy == nil {
		return fmt.Errorf("skill: %s: nil policy", stage.Operation)
	}
	facts := currentLeagueFacts(m)
	if facts.MainStoryComplete || stage.Done(facts) {
		return nil
	}
	if !stage.PredecessorDone(facts) {
		return gameruntime.NewProgressionPrerequisiteMissing(stage.Predecessor)
	}
	if err := leagueReachRoom(m, romData, policy, stage.RoomMap); err != nil {
		return fmt.Errorf("skill: %s: %w", stage.Operation, err)
	}
	if err := stage.Fight(m, romData, policy, stage); err != nil {
		return fmt.Errorf("skill: %s: %w", stage.Operation, err)
	}
	if facts := currentLeagueFacts(m); !facts.MainStoryComplete && !stage.Done(facts) {
		return fmt.Errorf("skill: %s: %s completion fact is still false", stage.Operation, stage.Name)
	}
	return nil
}
