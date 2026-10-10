package rom

import "testing"

// Prize = class base money × last party mon level (pic_pointers_money.asm,
// parties.asm): YOUNGSTER ¥15, BUG CATCHER ¥10.
func TestTrainerPrizeMoney(t *testing.T) {
	data := loadROM(t)
	for _, tc := range []struct {
		class, set uint8
		want       uint32
	}{
		{1, 1, 15 * 11}, // Route 3 Youngster: RATTATA, EKANS L11
		{1, 2, 15 * 14}, // Route 3 Youngster: SPEAROW L14
		{2, 1, 10 * 6},  // Viridian Forest Bug Catcher: L6
	} {
		got, err := TrainerPrizeMoney(data, tc.class, tc.set)
		if err != nil || got != tc.want {
			t.Errorf("TrainerPrizeMoney(%d,%d) = %d, %v; want %d", tc.class, tc.set, got, err, tc.want)
		}
	}
}
