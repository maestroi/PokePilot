package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func TestGSClairDefeatDoesNotGrantRisingBadge(t *testing.T) {
	if eventBeatClair != 1220 || eventIcePathWaterfall != 1672 ||
		eventDragonsDenDragonFang != 1704 {
		t.Fatal("late-Johto cartridge event bits drifted from pret/pokegold")
	}
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{}
		setGSEvent(mem, eventIcePathWaterfall)
		if !p.DecodeFirstBadgeProgress(mem).Has(ProgressIcePathWaterfallAcquired) {
			t.Fatal("Ice Path HM07 event not projected")
		}
		setGSEvent(mem, eventBeatClair)
		if !p.DecodeFirstBadgeProgress(mem).Has(ProgressClairDefeated) {
			t.Fatal("retail Clair battle event not projected")
		}
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressRisingBadgeEarned) {
			t.Fatal("Clair defeat incorrectly manufactured Rising Badge")
		}
		setGSEvent(mem, eventDragonsDenDragonFang)
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressRisingBadgeEarned) {
			t.Fatal("Dragon Fang disappearance without final award cannot satisfy eighth badge")
		}
		mem[sym.JohtoBadges] = 0x80
		if !p.DecodeFirstBadgeProgress(mem).Has(ProgressRisingBadgeEarned) {
			t.Fatal("ENGINE_RISINGBADGE bit not decoded")
		}
	}
}
