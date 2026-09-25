package skill

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

func leagueFactsReadyForStage(index int) state.StoryFacts {
	facts := state.StoryFacts{LeagueChallengeStarted: true}
	if index > 0 {
		facts.LeagueLoreleiDefeated = true
	}
	if index > 1 {
		facts.LeagueBrunoDefeated = true
	}
	if index > 2 {
		facts.LeagueAgathaDefeated = true
	}
	if index > 3 {
		facts.LeagueLanceDefeated = true
	}
	return facts
}

func leagueFactsCompletingStage(index int) state.StoryFacts {
	facts := leagueFactsReadyForStage(index)
	switch index {
	case 0:
		facts.LeagueLoreleiDefeated = true
	case 1:
		facts.LeagueBrunoDefeated = true
	case 2:
		facts.LeagueAgathaDefeated = true
	case 3:
		facts.LeagueLanceDefeated = true
	case 4:
		facts.LeagueChampionDefeated = true
	}
	return facts
}

func TestLeagueBattleStageDescriptorsDeclareOrderedSequence(t *testing.T) {
	want := []struct {
		name        string
		operation   string
		id          gameruntime.ProgressID
		predecessor gameruntime.ProgressID
		room        uint8
		homeX       uint8
		homeY       uint8
		nextRoom    uint8
		remaining   int
		waitBattle  bool
	}{
		{"Lorelei", "LeagueDefeatLorelei", leagueProgressLoreleiDefeated, leagueProgressChallengeStarted, loreleiRoomMap, 5, 2, brunoRoomMap, 4, false},
		{"Bruno", "LeagueDefeatBruno", leagueProgressBrunoDefeated, leagueProgressLoreleiDefeated, brunoRoomMap, 5, 2, agathaRoomMap, 3, false},
		{"Agatha", "LeagueDefeatAgatha", leagueProgressAgathaDefeated, leagueProgressBrunoDefeated, agathaRoomMap, 5, 2, lanceRoomMap, 2, false},
		{"Lance", "LeagueDefeatLance", leagueProgressLanceDefeated, leagueProgressAgathaDefeated, lanceRoomMap, 6, 1, championsRoomMap, 1, true},
		{"Champion", "LeagueDefeatChampion", leagueProgressChampionDefeated, leagueProgressLanceDefeated, championsRoomMap, 0, 0, 0, 0, false},
	}
	if len(leagueBattleStages) != len(want) {
		t.Fatalf("League stage count = %d, want %d", len(leagueBattleStages), len(want))
	}
	for i, expected := range want {
		stage := leagueBattleStages[i]
		if stage.Name != expected.name ||
			stage.Operation != expected.operation ||
			stage.ID != expected.id ||
			stage.Predecessor != expected.predecessor ||
			stage.RoomMap != expected.room ||
			stage.TrainerHomeX != expected.homeX ||
			stage.TrainerHomeY != expected.homeY {
			t.Fatalf("stage %d = %+v, want %+v", i, stage, expected)
		}

		if stage.Fight == nil || stage.Done == nil || stage.PredecessorDone == nil {
			t.Fatalf("stage %s is missing execution/fact hooks: %+v", stage.Name, stage)
		}
		if i == len(want)-1 {
			if stage.Exit != nil {
				t.Fatalf("Champion should keep ending behavior explicit, got exit %+v", stage.Exit)
			}
			continue
		}
		if stage.Exit == nil {
			t.Fatalf("stage %s has no declared exit", stage.Name)
		}
		if stage.Exit.NextRoom != expected.nextRoom ||
			stage.Exit.EncountersRemaining != expected.remaining ||
			stage.Exit.WaitForBattle != expected.waitBattle {
			t.Fatalf("stage %s exit = %+v, want next=%#02x remaining=%d wait=%v",
				stage.Name, stage.Exit, expected.nextRoom, expected.remaining, expected.waitBattle)
		}
	}
}

func TestLeagueStageDescriptorsOwnPredecessorAndCompletionFacts(t *testing.T) {
	for i, stage := range leagueBattleStages {
		before := leagueFactsReadyForStage(i)
		if !stage.PredecessorDone(before) {
			t.Fatalf("%s predecessor is false in its ready state: %+v", stage.Name, before)
		}
		if stage.Done(before) {
			t.Fatalf("%s completion is already true before its stage: %+v", stage.Name, before)
		}

		after := leagueFactsCompletingStage(i)
		if !stage.Done(after) {
			t.Fatalf("%s completion hook did not observe its committed fact: %+v", stage.Name, after)
		}

		if i > 0 {
			notReady := leagueFactsReadyForStage(i - 1)
			if stage.PredecessorDone(notReady) {
				t.Fatalf("%s predecessor accepted state before %s completed: %+v",
					stage.Name, leagueBattleStages[i-1].Name, notReady)
			}
			if stage.Predecessor != leagueBattleStages[i-1].ID {
				t.Fatalf("%s predecessor ID = %q, want %q",
					stage.Name, stage.Predecessor, leagueBattleStages[i-1].ID)
			}
		}
	}
}

func TestLeagueStageRoomLookupUsesDescriptorTable(t *testing.T) {
	for _, want := range leagueBattleStages {
		got, ok := leagueStageForRoom(want.RoomMap)
		if !ok {
			t.Fatalf("room %#02x has no League stage", want.RoomMap)
		}
		if got.ID != want.ID || got.Name != want.Name {
			t.Fatalf("room %#02x resolved %+v, want %+v", want.RoomMap, got, want)
		}
	}
	if _, ok := leagueStageForRoom(indigoPlateauLobbyMap); ok {
		t.Fatal("Indigo lobby must remain a checkpoint boundary, not a battle stage")
	}
}

func TestLeagueResumeSelectsEarliestIncompleteStage(t *testing.T) {
	for i, want := range leagueBattleStages {
		facts := leagueFactsReadyForStage(i)
		got, ok := earliestIncompleteLeagueStage(facts)
		if !ok {
			t.Fatalf("ready state for %s had no incomplete League stage", want.Name)
		}
		if got.ID != want.ID {
			t.Fatalf("ready state for %s resumed at %s (%q), want %q",
				want.Name, got.Name, got.ID, want.ID)
		}

		roomStage, ok := leagueStageForRoom(want.RoomMap)
		if !ok || roomStage.ID != got.ID {
			t.Fatalf("checkpoint room %#02x maps to %+v while semantic resume chose %+v",
				want.RoomMap, roomStage, got)
		}
	}

	complete := leagueFactsCompletingStage(len(leagueBattleStages) - 1)
	if got, ok := earliestIncompleteLeagueStage(complete); ok {
		t.Fatalf("fully completed League still selected stage %+v", got)
	}
}
