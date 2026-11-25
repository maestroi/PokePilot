package profile

import (
	"slices"
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func TestGoldSilverMilestonesSeparateHallOfFameFromRed(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{}
		setGSEvent(mem, eventBeatEliteFour)
		obs, err := p.DecodeObservation(mem, nil)
		if err != nil {
			t.Fatal(err)
		}
		// Champion stays credited after the League's battle flags reset.
		if !slices.Contains(obs.Milestones, "Hall of Fame") || !slices.Contains(obs.Milestones, "Champion") ||
			slices.Contains(obs.Milestones, "Red") {
			t.Fatalf("%s: Hall of Fame milestones = %v", p.ID(), obs.Milestones)
		}
		setGSEvent(mem, eventMtSilverOpened)
		setGSEvent(mem, eventMtSilverRedDisappeared)
		mem[sym.KantoBadges] = 0xff
		obs, err = p.DecodeObservation(mem, nil)
		if err != nil {
			t.Fatal(err)
		}
		if obs.Milestones[len(obs.Milestones)-1] != "Red" {
			t.Fatalf("%s: Red must be the final milestone: %v", p.ID(), obs.Milestones)
		}
	}
}
