package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

// These indices are derived from pret/pokegold constants/event_flags.asm.
// The receipt event survives handing the LOST_ITEM back to Copycat; it must
// not be interpreted as an item still present in the key-item pocket.
func TestGSRetainedCopycatEventsAndRealRailPass(t *testing.T) {
	if eventKantoCopycatMet != 207 || eventKantoDollReturned != 208 ||
		eventKantoRailPassReceived != 209 || eventKantoLostDollReceived != 210 ||
		gsRailPass != 0x86 {
		t.Fatalf("Copycat retail constants drift: %d %d %d %d pass=%#02x",
			eventKantoCopycatMet, eventKantoDollReturned, eventKantoRailPassReceived, eventKantoLostDollReceived, gsRailPass)
	}
	for _, profile := range []*Profile{NewGold(), NewSilver()} {
		var mem fakeMemory
		seen := profile.DecodeFirstBadgeProgress(&mem)
		for _, fact := range []game.ProgressID{
			ProgressKantoCopycatMet,
			ProgressKantoLostDollReceived,
			ProgressKantoRailPassReceived,
		} {
			if seen.Has(fact) {
				t.Fatalf("%s: empty save claims Copycat progress %s", profile.ID(), fact)
			}
		}
		gsSetTransitEvent(&mem, eventKantoCopycatMet)
		if !profile.DecodeFirstBadgeProgress(&mem).Has(ProgressKantoCopycatMet) {
			t.Fatalf("%s: Copycat request event did not project", profile.ID())
		}
		if profile.DecodeFirstBadgeProgress(&mem).Has(ProgressKantoLostDollReceived) {
			t.Fatalf("%s: hearing Copycat alone should not create the Doll", profile.ID())
		}
		gsSetTransitEvent(&mem, eventKantoLostDollReceived)
		if !profile.DecodeFirstBadgeProgress(&mem).Has(ProgressKantoLostDollReceived) {
			t.Fatalf("%s: Fan Club receipt event did not project", profile.ID())
		}
		gsSetTransitEvent(&mem, eventKantoDollReturned)
		// The Doll is consumed during this handoff. The receipt fact must
		// remain complete, so the planner never sends us back to the Fan Club.
		if !profile.DecodeFirstBadgeProgress(&mem).Has(ProgressKantoLostDollReceived) {
			t.Fatalf("%s: consumed Doll erased its durable story milestone", profile.ID())
		}
		gsSetTransitEvent(&mem, eventKantoRailPassReceived)
		if profile.DecodeFirstBadgeProgress(&mem).Has(ProgressKantoRailPassReceived) {
			t.Fatalf("%s: Pass gift flag with no PASS in bag falsely completed", profile.ID())
		}
		mem[sym.NumKeyItems] = 1
		mem[sym.KeyItems] = gsRailPass
		if !profile.DecodeFirstBadgeProgress(&mem).Has(ProgressKantoRailPassReceived) {
			t.Fatalf("%s: real Copycat PASS missing from story projection", profile.ID())
		}
		mem[sym.NumKeyItems] = 0
		if profile.DecodeFirstBadgeProgress(&mem).Has(ProgressKantoRailPassReceived) {
			t.Fatalf("%s: missing PASS was hidden by stale receipt event", profile.ID())
		}
	}
}

func TestGSMagnetTrainTransportRequiresPassAndPowerEvenAfterCopycat(t *testing.T) {
	for _, profile := range []*Profile{NewGold(), NewSilver()} {
		var mem fakeMemory
		gsSetTransitEvent(&mem, eventKantoCopycatMet)
		gsSetTransitEvent(&mem, eventKantoLostDollReceived)
		gsSetTransitEvent(&mem, eventKantoRailPassReceived)
		mem[sym.NumKeyItems] = 1
		mem[sym.KeyItems] = gsRailPass

		for _, from := range []game.RegionID{game.RegionJohto, game.RegionKanto} {
			option := gsTransport(t, profile.DecodeRegionTransports(&mem), "magnet_train", from)
			if option.Available || !gsHasMissing(option, gsCapKantoPower) {
				t.Fatalf("%s: PASS alone opened train from %s: %+v", profile.ID(), from, option)
			}
		}
		gsSetTransitEvent(&mem, gsEventRestoredKantoPower)
		for _, from := range []game.RegionID{game.RegionJohto, game.RegionKanto} {
			option := gsTransport(t, profile.DecodeRegionTransports(&mem), "magnet_train", from)
			if !option.Available || option.OneWay || !option.AnyDay {
				t.Fatalf("%s: real PASS + power did not enable daily bidirectional train from %s: %+v", profile.ID(), from, option)
			}
		}
	}
}
