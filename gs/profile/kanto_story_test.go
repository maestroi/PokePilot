package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestGoldSilverKantoProgressIsCartridgeVerified(t *testing.T) {
	badges := []struct {
		progress game.ProgressID
		mask byte
	}{
		{ProgressKantoBoulderBadgeEarned, kantoBadgeBoulderMask},
		{ProgressKantoCascadeBadgeEarned, kantoBadgeCascadeMask},
		{ProgressKantoThunderBadgeEarned, kantoBadgeThunderMask},
		{ProgressKantoRainbowBadgeEarned, kantoBadgeRainbowMask},
		{ProgressKantoSoulBadgeEarned, kantoBadgeSoulMask},
		{ProgressKantoMarshBadgeEarned, kantoBadgeMarshMask},
		{ProgressKantoVolcanoBadgeEarned, kantoBadgeVolcanoMask},
		{ProgressKantoEarthBadgeEarned, kantoBadgeEarthMask},
	}
	for _, profile := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{sym.JohtoBadges: 0xff}
		for _, badge := range badges {
			mem[sym.KantoBadges] = badge.mask
			story := profile.DecodeFirstBadgeProgress(mem)
			if !story.Has(badge.progress) || story.Has(ProgressKantoAllBadgesEarned) {
				t.Fatalf("%s: isolated Kanto badge %#02x missing or falsely all-eight: %+v",
					profile.ID(), badge.mask, story)
			}
		}
		mem[sym.KantoBadges] = 0xff
		if !profile.DecodeFirstBadgeProgress(mem).Has(ProgressKantoAllBadgesEarned) {
			t.Fatalf("%s: all eight Kanto badge bits did not complete full postgame goal", profile.ID())
		}
	}
}

func TestGoldSilverKantoPowerAndLeaderAvailabilityRequireActualEvents(t *testing.T) {
	if eventKantoReturnedMachinePart != 201 ||
		eventKantoMetPowerManager != 202 ||
		eventKantoMetRocketAtGym != 203 ||
		eventKantoPowerRestored != 205 ||
		eventKantoFoundMachinePart != 251 ||
		eventKantoTrainersAtGym != 1903 ||
		eventKantoBlueAtCinnabar != 1909 ||
		eventKantoBlueAtViridian != 1910 {
		t.Fatal("Kanto event indices drifted from vendored Gold/Silver source")
	}
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{}
		setGSEvent(mem, eventKantoTrainersAtGym)
		setGSEvent(mem, eventKantoBlueAtViridian)
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoMistyReturned) ||
			p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoBlueInvited) {
			t.Fatal("fresh save falsely reported available Misty or Blue")
		}
		setGSEvent(mem, eventKantoMetPowerManager)
		setGSEvent(mem, eventKantoFoundMachinePart)
		setGSEvent(mem, eventKantoReturnedMachinePart)
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoPowerRestored) {
			t.Fatal("returning part alone must not claim restored power")
		}
		setGSEvent(mem, eventKantoPowerRestored)
		if !p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoPowerRestored) {
			t.Fatal("missing restored-power fact")
		}
		// Misty remains away until the separate Route25 cutscene completes.
		setGSEvent(mem, eventKantoMetRocketAtGym)
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoMistyReturned) {
			t.Fatal("Misty returned while her Gym's visibility flag is still set")
		}
		mem[sym.EventFlags+eventKantoTrainersAtGym/8] &^= 1 << (eventKantoTrainersAtGym % 8)
		if !p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoMistyReturned) {
			t.Fatal("Route25's visible Misty did not project")
		}
		setGSEvent(mem, eventKantoBlueAtCinnabar)
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoBlueInvited) {
			t.Fatal("Blue still hidden at Viridian was counted as invited")
		}
		mem[sym.EventFlags+eventKantoBlueAtViridian/8] &^= 1 << (eventKantoBlueAtViridian % 8)
		if !p.DecodeFirstBadgeProgress(mem).Has(ProgressKantoBlueInvited) {
			t.Fatal("Blue's Cinnabar invitation did not project")
		}
	}
}
