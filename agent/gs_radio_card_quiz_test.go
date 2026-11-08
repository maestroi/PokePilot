package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSRadioCardQuizFollowsRetailSixChoices(t *testing.T) {
	// RadioTower1FRadioCardWomanScript:
	// accept quiz, Town Map YES, Nidorina YES, APRIKORN NO,
	// Magikarp TMs YES, Marie as Oak's cohost NO.
	want := [...]int{0, 0, 0, 1, 0, 1}
	if gsRadioQuizAnswers != want {
		t.Fatalf("Radio Tower quiz decisions = %v, want %v", gsRadioQuizAnswers, want)
	}
	if gsRadioQuizFrameBudget == 0 || gsRadioQuizMaxTextPresses == 0 {
		t.Fatal("Radio Card quiz lacks bounded frame or text-press limits")
	}
	room, err := gsOpeningMapID("RADIO_TOWER_1F")
	if err != nil || !gsSecondBadgeOwnedMap(room) {
		t.Fatalf("Radio Tower 1F not mapped and owned: %#04x err=%v", room, err)
	}
	if err := executeGSRadioCardQuiz(nil, nil); err == nil {
		t.Fatal("Radio Card quiz must reject nil emulator")
	}
}

func TestGoldSilverRadioCardIsAnExecutableGoldenrodMilestone(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		o := Objective{Kind: KindProgress, Progress: gsprofile.ProgressRadioCardReceived}
		if !gsSupportedProgress(o.Progress) || !adapter.SupportsObjective(o) {
			t.Fatalf("%s: Radio Card objective missing executor", id)
		}
		if got := gsObjectiveFrameBudget(o); got != gsFirstBadgeObjectiveFrameBudget {
			t.Fatalf("%s: Radio Card frame watchdog = %d", id, got)
		}
	}
}
