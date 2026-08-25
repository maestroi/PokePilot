package rom

import "testing"

func TestLookupMoveKnownEntries(t *testing.T) {
	romData := loadROM(t)
	for _, tc := range []struct {
		name          string
		id            uint8
		power, effect uint8
	}{
		{"SCRATCH", 10, 40, 0},
		{"TACKLE", 33, 35, 0},
		{"TAIL_WHIP", 39, 0, DefenseDown1Effect},
		{"GROWL", 45, 0, 18}, // ATTACK_DOWN1_EFFECT
		{"EMBER", 52, 40, 4},
	} {
		got, err := LookupMove(romData, tc.id)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.Power != tc.power {
			t.Errorf("%s power = %d, want %d", tc.name, got.Power, tc.power)
		}
		if got.Effect != tc.effect {
			t.Errorf("%s effect = %d, want %d", tc.name, got.Effect, tc.effect)
		}
	}
}

func TestLookupMoveRejectsEmptySlot(t *testing.T) {
	if _, err := LookupMove(loadROM(t), 0); err == nil {
		t.Fatal("LookupMove(0) returned no error; move id 0 is the empty slot")
	}
}
