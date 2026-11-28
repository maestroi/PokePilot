package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

// VermilionGymSurgeScript sets EVENT_BEAT_LTSURGE after the battle and
// ENGINE_THUNDERBADGE after the badge award text begins. Only the second
// fact proves the first Kanto Gym is complete.
func TestGSKantoThunderBadgeRequiresAwardFlag(t *testing.T) {
	if eventBeatLtSurge != 1223 {
		t.Fatalf("EVENT_BEAT_LTSURGE index = %d, want 1223", eventBeatLtSurge)
	}
	if kantoBadgeThunderMask != 1<<2 {
		t.Fatalf("ENGINE_THUNDERBADGE mask = %#02x, want 0x04", kantoBadgeThunderMask)
	}
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		t.Run(string(p.ID()), func(t *testing.T) {
			mem := fakeGSReader{}
			setGSEvent(mem, eventBeatEliteFour)
			setGSEvent(mem, gsEventFirstShipTrip)
			setGSEvent(mem, eventBeatLtSurge)
			if p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoThunderBadgeEarned) {
				t.Fatal("Lt. Surge win event alone claimed Thunder Badge")
			}
			mem[sym.JohtoBadges] = 0xff
			if p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoThunderBadgeEarned) {
				t.Fatal("all Johto badges falsely counted as Kanto Thunder Badge")
			}
			mem[sym.KantoBadges] = 1 << 0 // Boulder, not Thunder
			if p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoThunderBadgeEarned) {
				t.Fatal("Boulder Badge falsely counted as Thunder Badge")
			}
			mem[sym.KantoBadges] = kantoBadgeThunderMask
			if !p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoThunderBadgeEarned) {
				t.Fatal("Thunder Badge engine bit did not complete Kanto milestone")
			}
			if mem[sym.JohtoBadges] != 0xff {
				t.Fatal("Kanto badge verifier modified Johto progress")
			}
		})
	}
}

func TestGSKantoThunderBadgeIsObservableWithoutOtherKantoBadges(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{}
		mem[sym.KantoBadges] = kantoBadgeThunderMask
		obs, err := p.DecodeObservation(mem, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !obs.Story.Has(ProgressKantoThunderBadgeEarned) {
			t.Fatalf("%s: Thunder Badge absent from portable story facts", p.ID())
		}
		if fact, ok := obs.Story.Lookup(ProgressKantoBadges); !ok || fact.Value != 1 || fact.Complete {
			t.Fatalf("%s: Kanto total should be exactly 1/8: %+v present=%v", p.ID(), fact, ok)
		}
		if len(obs.Badges) != 1 || obs.Badges[0] != "thunder" {
			t.Fatalf("%s: retail badge projection = %v, want [thunder]", p.ID(), obs.Badges)
		}
	}
}
