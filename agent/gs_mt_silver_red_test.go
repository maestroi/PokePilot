package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSMtSilverRedObjectivesHaveBoundedExecutors(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		for _, fact := range []ProgressID{
			gsprofile.ProgressMtSilverOpened,
			gsprofile.ProgressMtSilverRedDefeated,
		} {
			o := Objective{Kind: KindProgress, Progress: fact}
			if !gsSupportedProgress(fact) || !adapter.SupportsObjective(o) {
				t.Fatalf("%s: missing Mt. Silver executor %s", id, fact)
			}
			if got := gsObjectiveFrameBudget(o); got != gsFirstBadgeObjectiveFrameBudget {
				t.Fatalf("%s: budget for %s = %d", id, fact, got)
			}
		}
	}
	if err := executeGSMtSilverPermit(nil, nil); err == nil {
		t.Fatal("Oak permit executed without a machine")
	}
	if err := executeGSMtSilverRed(nil, nil); err == nil {
		t.Fatal("Red ending executed without a machine")
	}
	for _, name := range []string{
		"PALLET_TOWN", "OAKS_LAB", "ROUTE_22", "ROUTE_28",
		"SILVER_CAVE_OUTSIDE", "SILVER_CAVE_POKECENTER_1F",
		"SILVER_CAVE_ROOM_1", "SILVER_CAVE_ROOM_2", "SILVER_CAVE_ROOM_3",
	} {
		id, err := gsOpeningMapID(name)
		if err != nil || !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("%s not in the Red approach corridor: map=%#04x err=%v", name, id, err)
		}
	}
}

func TestGSRedEndingPostconditionRequiresEventAfterOakAndNoBattle(t *testing.T) {
	for _, gameID := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		start := Observation{
			GameID: gameID, Story: ProgressState{
				{ID: gsprofile.ProgressKantoAllBadgesEarned, Complete: true},
				{ID: gsprofile.ProgressMtSilverOpened, Complete: true},
			},
		}
		after := start
		after.Story = append(append(ProgressState{}, start.Story...),
			ProgressFact{ID: gsprofile.ProgressMtSilverRedDefeated, Complete: true})
		if err := gsVerifyRedEndingEvent(start, after); err != nil {
			t.Fatalf("%s: real Red disappearance event rejected: %v", gameID, err)
		}
		battle := after
		battle.InBattle = true
		if err := gsVerifyRedEndingEvent(start, battle); !errors.Is(err, ErrObjectivePostconditionFailed) {
			t.Fatalf("%s: battle-in-progress must not be victory: %v", gameID, err)
		}
		if err := gsVerifyRedEndingEvent(start, start); !errors.Is(err, ErrObjectivePostconditionFailed) {
			t.Fatalf("%s: Red must be observed as a new event: %v", gameID, err)
		}
		already := after
		if err := gsVerifyRedEndingEvent(already, after); !errors.Is(err, ErrObjectivePostconditionFailed) {
			t.Fatalf("%s: preexisting Red victory must not count as a new one: %v", gameID, err)
		}
		noPermit := start
		noPermit.Story = ProgressState{{ID: gsprofile.ProgressKantoAllBadgesEarned, Complete: true}}
		if err := gsVerifyRedEndingEvent(noPermit, after); !errors.Is(err, ErrObjectivePostconditionFailed) {
			t.Fatalf("%s: cannot skip Oak's Mt. Silver gate: %v", gameID, err)
		}
	}
}
