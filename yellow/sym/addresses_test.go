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
		"wCurMapHeight":             {CurMapHeight, 0xD367},
		"wCurMapWidth":              {CurMapWidth, 0xD368},
		"wPlayerName":               {PlayerName, 0xD157},
		"wRivalName":                {RivalName, 0xD349},
		"wSpritePlayerStateData1+9": {SpritePlayerFacing, 0xC109},
		"wTileMap":                  {TileMap, 0xC3A0},
		"wCurrentMenuItem":          {CurrentMenuItem, 0xCC26},
		"wMaxMenuItem":              {MaxMenuItem, 0xCC28},
		"wFontLoaded":               {FontLoaded, 0xCFC3},
		"wWalkCounter":              {WalkCounter, 0xCFC4},
		"wJoyIgnore":                {JoyIgnore, 0xCD6B},
		"wPartyCount":               {PartyCount, 0xD162},
		"wPartyMon1":                {PartyMon1, 0xD16A},
		"wIsInBattle":               {IsInBattle, 0xD056},
		"wNumBagItems":              {NumBagItems, 0xD31C},
		"wBagItems":                 {BagItems, 0xD31D},
		"wPlayerMoney":              {PlayerMoney, 0xD346},
		"wObtainedBadges":           {ObtainedBadges, 0xD355},
		"wStatusFlags1":              {StatusFlags1, 0xD727},
		"wStatusFlags4":              {StatusFlags4, 0xD72D},
		"wElite4Flags":               {Elite4Flags, 0xD733},
		"wEventFlags":                {EventFlags, 0xD746},
		"wRivalStarter":              {RivalStarter, 0xD714},
		"wPlayerStarter":             {PlayerStarter, 0xD716},
		"wLastBlackoutMap":           {LastBlackoutMap, 0xD718},
		"wPikachuHappiness":          {PikachuHappiness, 0xD46F},
		"wPikachuMood":               {PikachuMood, 0xD470},
		"wPikachuSpawnStateFlags":    {PikachuSpawnStateFlags, 0xD471},
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
