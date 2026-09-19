package sym

import "testing"

func TestPhase0AddressesMatchSupportedYellowLayout(t *testing.T) {
	tests := map[string]struct {
		got  uint16
		want uint16
	}{
		"wCurMap":                   {CurMap, 0xD35D},
		"wYCoord":                   {YCoord, 0xD360},
		"wXCoord":                   {XCoord, 0xD361},
		"wSpritePlayerStateData1+9": {SpritePlayerFacing, 0xC109},
		"wPartyCount":               {PartyCount, 0xD162},
		"wPartyMon1":                {PartyMon1, 0xD16A},
		"wIsInBattle":               {IsInBattle, 0xD056},
		"wNumBagItems":              {NumBagItems, 0xD31C},
		"wBagItems":                 {BagItems, 0xD31D},
		"wPlayerMoney":              {PlayerMoney, 0xD346},
		"wObtainedBadges":           {ObtainedBadges, 0xD355},
	}
	for name, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = 0x%04x, want 0x%04x", name, tc.got, tc.want)
		}
	}
	if PartyMonSize != 0x2C {
		t.Fatalf("PartyMonSize = %d, want 44", PartyMonSize)
	}
}
