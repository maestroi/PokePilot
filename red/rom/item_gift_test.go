package rom

import "testing"

func TestItemGiftActors(t *testing.T) {
	data := loadROM(t)
	for _, tc := range []struct {
		name  string
		mapID uint8
		x, y  uint8
		item  uint8
	}{
		{"Mr. Psychic TM29 (bit b,a)", 0xB7, 5, 3, 0xE5},
		{"Route 12 Gate TM39 (CheckEvent ..., 1 rrca form)", 0xC3, 3, 4, 0xEF},
		{"Viridian Fisher TM42, not the sleepy man before him", 0x01, 6, 23, 0xF2},
	} {
		gifts, err := ItemGiftActors(data, tc.mapID)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(gifts) != 1 || gifts[0].X != tc.x || gifts[0].Y != tc.y || gifts[0].Item != tc.item || gifts[0].EventMask == 0 {
			t.Errorf("%s: gifts = %+v", tc.name, gifts)
		}
	}
}
