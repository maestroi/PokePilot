package agent

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
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

// TestGSRadioQuizStandIsReachableAcrossCounter guards triage:4effb96e547d00fd:
// the quiz staged from (12,7), inside the counter enclosure, so routing from
// the Radio Tower door always failed with "no path".
func TestGSRadioQuizStandIsReachableAcrossCounter(t *testing.T) {
	if gsRadioQuizStandX != gsRadioQuizCounterX || gsRadioQuizStandY+1 != gsRadioQuizCounterY {
		t.Fatalf("stand (%d,%d) is not the tile north of the counter (%d,%d)",
			gsRadioQuizStandX, gsRadioQuizStandY, gsRadioQuizCounterX, gsRadioQuizCounterY)
	}
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("set POKEMON_GOLD_ROM to check the quiz stand against the live grid")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := gsrom.NewWorldProvider(romData)
	if err != nil {
		t.Fatal(err)
	}
	tower, err := gsOpeningMapID("RADIO_TOWER_1F")
	if err != nil {
		t.Fatal(err)
	}
	spec, err := provider.Grid(tower, nil, worldmodel.TraversalMode(0))
	if err != nil {
		t.Fatal(err)
	}
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	if grid.Walkable(int(gsRadioQuizCounterX), int(gsRadioQuizCounterY)) {
		t.Fatalf("counter (%d,%d) decoded walkable", gsRadioQuizCounterX, gsRadioQuizCounterY)
	}
	// Flood from the entrance door (2,7) to the stand tile.
	type pt struct{ x, y int }
	seen := map[pt]bool{{2, 7}: true}
	queue := []pt{{2, 7}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, d := range []pt{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := pt{p.x + d.x, p.y + d.y}
			if !seen[n] && grid.Walkable(n.x, n.y) {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	if !seen[pt{int(gsRadioQuizStandX), int(gsRadioQuizStandY)}] {
		t.Fatalf("quiz stand (%d,%d) unreachable from the door", gsRadioQuizStandX, gsRadioQuizStandY)
	}
}
