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
		"wTopMenuItemX":             {TopMenuItemX, 0xCC25},
		"wTopMenuItemY":             {TopMenuItemY, 0xCC24},
		"wMenuJoypadPollCount":      {MenuJoypadPollCount, 0xCC34},
		"wMenuWatchedKeys":          {MenuWatchedKeys, 0xCC29},
		"wItemQuantity":             {ItemQuantity, 0xCF95},
		"hMoney":                    {MoneyTemp, 0xFF9F},
		"wListScrollOffset":         {ListScrollOffset, 0xCC36},
		"wListCount":                {ListCount, 0xD129},
		"wListMenuID":               {ListMenuID, 0xCF93},
		"wPartyMenuTypeOrMessageID": {PartyMenuTypeOrMessage, 0xD07C},
		"wFieldMoves":               {FieldMoves, 0xCD3D},
		"wFlyLocationsList":         {FlyLocationsList, 0xCD3E},
		"wDestinationMap":           {DestinationMap, 0xD719},

		"wActionResultOrTookBattleTurn": {ActionResult, 0xCD6A},

		"wTileInFrontOfPlayer":      {TileInFrontOfPlayer, 0xCFC5},
		"wWalkBikeSurfState":        {WalkBikeSurfState, 0xD6FF},
		"wMapPalOffset":             {MapPalOffset, 0xD35C},
		"wMoveMenuType":             {MoveMenuType, 0xCCDB},
		"wNumMovesMinusOne":         {NumMovesMinusOne, 0xCD6C},
		"wForcePlayerToChooseMon":   {ForcePlayerToChooseMon, 0xD11E},
		"wBattleMonSpecies":         {BattleMonSpecies, 0xD013},
		"wBattleMonHP":              {BattleMonHP, 0xD014},
		"wBattleMonMoves":           {BattleMonMoves, 0xD01B},
		"wBattleMonLevel":           {BattleMonLevel, 0xD021},
		"wBattleMonMaxHP":           {BattleMonMaxHP, 0xD022},
		"wBattleMonPP":              {BattleMonPP, 0xD02C},
		"wEnemyMonSpecies":          {EnemyMonSpecies, 0xCFE4},
		"wEnemyMonHP":               {EnemyMonHP, 0xCFE5},
		"wEnemyMonLevel":            {EnemyMonLevel, 0xCFF2},
		"wEnemyMonMaxHP":            {EnemyMonMaxHP, 0xCFF3},
		"wBattleResult":             {BattleResult, 0xCF0B},
		"wPlayerMonNumber":          {PlayerMonNumber, 0xCC2F},
		"wWhichPokemon":             {WhichPokemon, 0xCF91},
		"wMoveNum":                  {MoveNum, 0xD0DF},
		"wPlayerMoveNum":            {PlayerMoveNum, 0xCFD1},
		"wPlayerSelectedMove":       {PlayerSelectedMove, 0xCCDC},
		"wEnemyMoveNum":             {EnemyMoveNum, 0xCFCB},
		"wTrainerClass":             {TrainerClass, 0xD030},
		"wTrainerNo":                {TrainerNo, 0xD05C},
		"wBattleType":               {BattleType, 0xD059},
		"wCurOpponent":              {CurOpponent, 0xD058},
		"wEnemyMonPartyPos":         {EnemyMonPartyPos, 0xCFE7},
		"wIsInBattle":               {IsInBattle, 0xD056},
		"wPokedexOwned":             {PokedexOwned, 0xD2F6},
		"wPokedexSeen":              {PokedexSeen, 0xD309},
		"wNumBagItems":              {NumBagItems, 0xD31C},
		"wBagItems":                 {BagItems, 0xD31D},
		"wPlayerMoney":              {PlayerMoney, 0xD346},
		"wObtainedBadges":           {ObtainedBadges, 0xD355},
		"wStatusFlags1":             {StatusFlags1, 0xD727},
		"wStatusFlags4":             {StatusFlags4, 0xD72D},
		"wElite4Flags":              {Elite4Flags, 0xD733},
		"wEventFlags":               {EventFlags, 0xD746},
		"wRivalStarter":             {RivalStarter, 0xD714},
		"wPlayerStarter":            {PlayerStarter, 0xD716},
		"wLastBlackoutMap":          {LastBlackoutMap, 0xD718},
		"wPikachuHappiness":         {PikachuHappiness, 0xD46F},
		"wPikachuMood":              {PikachuMood, 0xD470},
		"wPikachuSpawnStateFlags":   {PikachuSpawnStateFlags, 0xD471},
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
