package rom

import "testing"

// The ROM chart must agree with the decomp on Gen 2's additions and on the
// cases the Gen 1 chart got differently.
func TestTypeEffectivenessReadsTheROMChart(t *testing.T) {
	rom := loadGold(t)
	const (
		normal, fighting, ground, rock, ghost, steel = 0x00, 0x01, 0x04, 0x05, 0x08, 0x09
		fire, water, electric, psychic, ice, dark    = 0x14, 0x15, 0x17, 0x18, 0x19, 0x1b
	)
	for _, tc := range []struct {
		move, d1, d2 uint8
		want         int
	}{
		{electric, ground, ground, 0},
		{water, rock, ground, 40},
		{ghost, psychic, psychic, 20}, // Gen 1's ROM had this immune
		{dark, psychic, psychic, 20},
		{fire, steel, steel, 20},
		{normal, ghost, ghost, 0}, // a Foresight row, applied in ordinary battles
		{fighting, normal, normal, 20},
		{ice, water, water, 5},
		{psychic, dark, dark, 0},
	} {
		got, err := TypeEffectiveness(rom, tc.move, tc.d1, tc.d2)
		if err != nil || got != tc.want {
			t.Errorf("%#x vs %#x/%#x = %d, %v; want %d", tc.move, tc.d1, tc.d2, got, err, tc.want)
		}
	}
}
