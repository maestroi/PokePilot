package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestGoldSilverCampaignBadgeCountExcludesKantoBadges(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		t.Run(string(p.ID()), func(t *testing.T) {
			mem := fakeGSReader{}
			mem[sym.JohtoBadges] = 0x3f
			mem[sym.KantoBadges] = 0xff
			obs, err := p.DecodeObservation(mem, nil)
			if err != nil {
				t.Fatal(err)
			}
			fact, ok := obs.Story.Lookup(game.ProgressCampaignBadges)
			if !ok || fact.Value != 6 || fact.Complete {
				t.Fatalf("six Johto plus eight Kanto badges must count six campaign badges: %+v, present=%v", fact, ok)
			}
			if len(obs.Badges) != 14 {
				t.Fatalf("expected all earned badges in ordinary observation, got %v", obs.Badges)
			}
			mem[sym.JohtoBadges] = 0xff
			obs, err = p.DecodeObservation(mem, nil)
			if err != nil {
				t.Fatal(err)
			}
			fact, ok = obs.Story.Lookup(game.ProgressCampaignBadges)
			if !ok || fact.Value != 8 || !fact.Complete {
				t.Fatalf("all Johto badges should complete the eight-badge stage: %+v, present=%v", fact, ok)
			}
		})
	}
}
