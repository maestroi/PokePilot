package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSKantoSurgeControllerIsExecutableAndBounded(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		objective := Objective{Kind: KindProgress, Progress: gsprofile.ProgressKantoThunderBadgeEarned}
		if !gsSupportedProgress(objective.Progress) || !adapter.SupportsObjective(objective) {
			t.Fatalf("%s cannot execute its first Kanto badge objective", id)
		}
		if budget := gsObjectiveFrameBudget(objective); budget != gsFirstBadgeObjectiveFrameBudget {
			t.Fatalf("%s: Thunder Badge budget = %d, want %d", id, budget, gsFirstBadgeObjectiveFrameBudget)
		}
	}
	if err := executeGSKantoSurge(nil, nil); err == nil {
		t.Fatal("nil emulator cannot manufacture Lt. Surge victory")
	}
}

func TestGSKantoSurgeOnlyOwnsVermilionApproach(t *testing.T) {
	for _, name := range []string{
		"VERMILION_PORT", "VERMILION_PORT_PASSAGE",
		"VERMILION_CITY", "VERMILION_POKECENTER_1F", "VERMILION_GYM",
	} {
		id, err := gsOpeningMapID(name)
		if err != nil {
			t.Fatalf("map %s missing from Gold/Silver native catalog: %v", name, err)
		}
		if !gsSurgeApproachMap(id) || !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("Kanto story has no owned path through %s", name)
		}
	}
	for _, name := range []string{"OLIVINE_CITY", "OLIVINE_PORT", "WILLS_ROOM", "PEWTER_CITY"} {
		id, err := gsOpeningMapID(name)
		if err != nil {
			t.Fatalf("map %s: %v", name, err)
		}
		if gsSurgeApproachMap(id) {
			t.Fatalf("Surge's local story executor incorrectly owns %s", name)
		}
	}
}

func TestGSKantoSurgeProgressionRequiresRealLanding(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		obs := Observation{
			GameID: id, PartyCount: 1,
			Story: ProgressState{{ID: gsprofile.ProgressStarterReceived, Complete: true}},
		}
		// Walk the same catalog chain as a full Johto campaign instead of
		// manufacturing a partial late-game observation that still lacks the
		// early Johto events the normal planner correctly requires.
		reachedLanding := false
		for i := 0; i < 100; i++ {
			got := adapter.ProgressionObjectives(obs)
			if len(got) != 1 || got[0].Kind != KindProgress {
				t.Fatalf("%s: unsupported stage %d while reaching Kanto: %+v", id, i, got)
			}
			if got[0].Progress == gsprofile.ProgressKantoFirstLanding {
				reachedLanding = true
				break
			}
			obs.Story = append(obs.Story, ProgressFact{ID: got[0].Progress, Complete: true})
		}
		if !reachedLanding {
			t.Fatalf("%s: campaign catalog did not reach the first Kanto landing", id)
		}
		obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressKantoFirstLanding, Complete: true})
		if got := adapter.ProgressionObjectives(obs); len(got) != 1 ||
			got[0].Progress != gsprofile.ProgressKantoThunderBadgeEarned {
			t.Fatalf("%s: Kanto landing did not offer Lt. Surge: %+v", id, got)
		}
		obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressKantoThunderBadgeEarned, Complete: true})
		if got := adapter.ProgressionObjectives(obs); len(got) != 0 {
			t.Fatalf("%s: completed Thunder Badge offered unowned Kanto milestone: %+v", id, got)
		}
	}
}
