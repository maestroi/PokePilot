package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func TestGSStrengthPitMilestonesNeedExactRetailFlags(t *testing.T) {
	if eventBlackthornGymBoulder1 != 1798 || eventIcePathBoulder1 != 1801 ||
		eventIcePathFloorBoulder1 != 1805 {
		t.Fatal("native boulder event IDs drifted from pokegold")
	}
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{}
		for n := uint16(0); n < 4; n++ {
			// Initial lower-floor stones are hidden; only a retail boulder
			// fall may clear their corresponding hidden event bit.
			setGSEvent(mem, eventIcePathFloorBoulder1+n)
		}
		for n := uint16(0); n < 3; n++ {
			setGSEvent(mem, eventIcePathBoulder1+n)
			setGSEvent(mem, eventBlackthornGymBoulder1+n)
		}
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressIcePathBouldersDropped) ||
			!p.DecodeFirstBadgeProgress(mem).Has(ProgressBlackthornGymBouldersDropped) {
			t.Fatal("incomplete Ice Path cannot be conflated with completed Gym pits")
		}
		setGSEvent(mem, eventIcePathBoulder1+3)
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressIcePathBouldersDropped) {
			t.Fatal("source boulders alone must not forge lower-floor holes")
		}
		for n := uint16(0); n < 4; n++ {
			event := eventIcePathFloorBoulder1 + n
			addr := sym.EventFlags + event/8
			mem[addr] &^= byte(1 << uint(event%8))
		}
		if !p.DecodeFirstBadgeProgress(mem).Has(ProgressIcePathBouldersDropped) {
			t.Fatal("Ice Path full four-boulder fall was not projected")
		}
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressRisingBadgeEarned) {
			t.Fatal("Strength pit puzzle did not award Rising Badge")
		}
	}
}
