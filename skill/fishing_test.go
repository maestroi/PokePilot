package skill

import "testing"

func TestFishingRodItemRecognizesOnlyRods(t *testing.T) {
	for _, item := range []uint8{itemOldRod, itemGoodRod, itemSuperRod} {
		if !fishingRodItem(item) {
			t.Fatalf("rod %#02x was not recognized", item)
		}
	}
	for _, item := range []uint8{0x04, 0x14, 0x49, 0x4f} {
		if fishingRodItem(item) {
			t.Fatalf("non-rod %#02x was recognized as a rod", item)
		}
	}
}
