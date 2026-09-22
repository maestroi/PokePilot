package sym

import "testing"

func TestPhase0AddressesMatchSupportedYellowLayout(t *testing.T) {
	tests := []struct {
		name string
		got  uint16
		want uint16
	}{
		{name: "wCurMap", got: CurMap, want: 0xD35D},
		{name: "wYCoord", got: YCoord, want: 0xD360},
		{name: "wXCoord", got: XCoord, want: 0xD361},
		{name: "wCurMapHeight", got: CurMapHeight, want: 0xD367},
		{name: "wCurMapWidth", got: CurMapWidth, want: 0xD368},
		{name: "wPlayerName", got: PlayerName, want: 0xD157},
		{name: "wRivalName", got: RivalName, want: 0xD349},
		{name: "wSpritePlayerStateData1", got: SpritePlayerStateData1, want: 0xC100},
		{name: "wSpriteStateData2", got: SpriteStateData2, want: 0xC200},
		{name: "wSpritePlayerStateData1+9", got: SpritePlayerFacing, want: 0xC109},
		{name: "wOverworldMap", got: OverworldMap, want: 0xC6E8},
		{name: "wTileMap", got: TileMap, want: 0xC3A0},
		{name: "wCurrentMenuItem", got: CurrentMenuItem, want: 0xCC26},
		{name: "wMaxMenuItem", got: MaxMenuItem, want: 0xCC28},
		{name: "wFontLoaded", got: FontLoaded, want: 0xCFC3},
		{name: "wWalkCounter", got: WalkCounter, want: 0xCFC4},
		{name: "wJoyIgnore", got: JoyIgnore, want: 0xCD6B},
		{name: "wPartyCount", got: PartyCount, want: 0xD162},
		{name: "wPartyMon1", got: PartyMon1, want: 0xD16A},
		{name: "wTopMenuItemX", got: TopMenuItemX, want: 0xCC25},
		{name: "wTopMenuItemY", got: TopMenuItemY, want: 0xCC24},
		{name: "wMenuJoypadPollCount", got: MenuJoypadPollCount, want: 0xCC34},
		{name: "wMenuWatchedKeys", got: MenuWatchedKeys, want: 0xCC29},
		{name: "wItemQuantity", got: ItemQuantity, want: 0xCF95},
		{name: "hMoney", got: MoneyTemp, want: 0xFF9F},
		{name: "wListScrollOffset", got: ListScrollOffset, want: 0xCC36},
		{name: "wListCount", got: ListCount, want: 0xD129},
		{name: "wListMenuID", got: ListMenuID, want: 0xCF93},
		{name: "wPartyMenuTypeOrMessageID", got: PartyMenuTypeOrMessage, want: 0xD07C},
		{name: "wFieldMoves", got: FieldMoves, want: 0xCD3D},
		{name: "wRodResponse", got: RodResponse, want: 0xCD3D},
		{name: "wFlyLocationsList", got: FlyLocationsList, want: 0xCD3E},
		{name: "wDestinationMap", got: DestinationMap, want: 0xD719},
		{name: "wActionResultOrTookBattleTurn", got: ActionResult, want: 0xCD6A},
		{name: "wTileInFrontOfPlayer", got: TileInFrontOfPlayer, want: 0xCFC5},
		{name: "wWalkBikeSurfState", got: WalkBikeSurfState, want: 0xD6FF},
		{name: "wNumSafariBalls", got: NumSafariBalls, want: 0xDA46},
		{name: "wSafariSteps", got: SafariSteps, want: 0xD70C},
		{name: "wMapPalOffset", got: MapPalOffset, want: 0xD35C},
		{name: "wMoveMenuType", got: MoveMenuType, want: 0xCCDB},
		{name: "wNumMovesMinusOne", got: NumMovesMinusOne, want: 0xCD6C},
		{name: "wForcePlayerToChooseMon", got: ForcePlayerToChooseMon, want: 0xD11E},
		{name: "wBattleMonSpecies", got: BattleMonSpecies, want: 0xD013},
		{name: "wBattleMonHP", got: BattleMonHP, want: 0xD014},
		{name: "wBattleMonMoves", got: BattleMonMoves, want: 0xD01B},
		{name: "wBattleMonLevel", got: BattleMonLevel, want: 0xD021},
		{name: "wBattleMonMaxHP", got: BattleMonMaxHP, want: 0xD022},
		{name: "wBattleMonPP", got: BattleMonPP, want: 0xD02C},
		{name: "wEnemyMonSpecies", got: EnemyMonSpecies, want: 0xCFE4},
		{name: "wEnemyMonHP", got: EnemyMonHP, want: 0xCFE5},
		{name: "wEnemyMonLevel", got: EnemyMonLevel, want: 0xCFF2},
		{name: "wEnemyMonMaxHP", got: EnemyMonMaxHP, want: 0xCFF3},
		{name: "wBattleResult", got: BattleResult, want: 0xCF0B},
		{name: "wPlayerMonNumber", got: PlayerMonNumber, want: 0xCC2F},
		{name: "wWhichPokemon", got: WhichPokemon, want: 0xCF91},
		{name: "wMoveNum", got: MoveNum, want: 0xD0DF},
		{name: "wPlayerMoveNum", got: PlayerMoveNum, want: 0xCFD1},
		{name: "wPlayerSelectedMove", got: PlayerSelectedMove, want: 0xCCDC},
		{name: "wEnemyMoveNum", got: EnemyMoveNum, want: 0xCFCB},
		{name: "wTrainerClass", got: TrainerClass, want: 0xD030},
		{name: "wTrainerNo", got: TrainerNo, want: 0xD05C},
		{name: "wBattleType", got: BattleType, want: 0xD059},
		{name: "wCurOpponent", got: CurOpponent, want: 0xD058},
		{name: "wEnemyMonPartyPos", got: EnemyMonPartyPos, want: 0xCFE7},
		{name: "wIsInBattle", got: IsInBattle, want: 0xD056},
		{name: "wPokedexOwned", got: PokedexOwned, want: 0xD2F6},
		{name: "wPokedexSeen", got: PokedexSeen, want: 0xD309},
		{name: "wNumBagItems", got: NumBagItems, want: 0xD31C},
		{name: "wBagItems", got: BagItems, want: 0xD31D},
		{name: "wPlayerMoney", got: PlayerMoney, want: 0xD346},
		{name: "wObtainedBadges", got: ObtainedBadges, want: 0xD355},
		{name: "wStatusFlags1", got: StatusFlags1, want: 0xD727},
		{name: "wStatusFlags4", got: StatusFlags4, want: 0xD72D},
		{name: "wElite4Flags", got: Elite4Flags, want: 0xD733},
		{name: "wFirstLockTrashCanIndex", got: FirstLockTrashCanIndex, want: 0xD743},
		{name: "wSecondLockTrashCanIndex", got: SecondLockTrashCanIndex, want: 0xD744},
		{name: "wEventFlags", got: EventFlags, want: 0xD746},
		{name: "wRivalStarter", got: RivalStarter, want: 0xD714},
		{name: "wPlayerStarter", got: PlayerStarter, want: 0xD716},
		{name: "wLastBlackoutMap", got: LastBlackoutMap, want: 0xD718},
		{name: "wPikachuHappiness", got: PikachuHappiness, want: 0xD46F},
		{name: "wPikachuMood", got: PikachuMood, want: 0xD470},
		{name: "wPikachuSpawnStateFlags", got: PikachuSpawnStateFlags, want: 0xD471},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = 0x%04x, want 0x%04x", tc.name, tc.got, tc.want)
		}
	}
	if PartyMonSize != 0x2C {
		t.Fatalf("PartyMonSize = %d, want 44", PartyMonSize)
	}
	if OverworldMapLen != 1300 {
		t.Fatalf("OverworldMapLen = %d, want 1300", OverworldMapLen)
	}
}
