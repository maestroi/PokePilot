package sym

import "testing"

func TestRetailGoldSilverLiveWRAMLayout(t *testing.T) {
	tests := []struct {
		name string
		got  uint16
		want uint16
	}{
		{"wOverworldMapBlocks", OverworldMap, 0xC700},
		{"wPlayerStepFlags", PlayerStepFlags, 0xCE85},
		{"wMapHeight", MapHeight, 0xD087},
		{"wMapWidth", MapWidth, 0xD088},
		{"wMapStatus", MapStatus, 0xD159},
		{"wMapEventStatus", MapEventStatus, 0xD15A},
		{"wScriptFlags", ScriptFlags, 0xD15B},
		{"wScriptMode", ScriptMode, 0xD15E},
		{"wScriptRunning", ScriptRunning, 0xD15F},
		{"wObjectStructs", ObjectStructs, 0xD1FD},
		{"wPlayerState", PlayerState, 0xD682},
		{"wMapGroup", MapGroup, 0xDA00},
		{"wPartyCount", PartyCount, 0xDA22},
		{"wBattleMode", BattleMode, 0xD116},
		{"wBattleType", BattleType, 0xD119},
		{"wForcedSwitch", ForcedSwitch, 0xD11C},
		{"wMoveSelectionMenuType", MoveSelectionMenuType, 0xD11F},
		{"wBattleMenuCursorPosition", BattleMenuCursor, 0xCFC4},
		{"wCurBattleMon", CurBattleMon, 0xCFC6},
		{"wCurMoveNum", CurMoveNum, 0xCFC7},
		{"wNumMoves", NumMoves, 0xCFE3},
		{"wBattlePlayerAction", BattlePlayerAction, 0xCFE4},
		{"wCurPartyMon", CurPartyMon, 0xD005},
		{"wPutativeTMHMMove", PutativeTMHMMove, 0xD14D},
		{"wTextboxFlags", TextboxFlags, 0xD19C},
		{"wTileMap", TileMap, 0xC3A0},
		{"wBattleMon", BattleMon, 0xCB0C},
		{"wPlayerStatLevels", PlayerStatLevels, 0xCBAA},
		{"wEnemyStatLevels", EnemyStatLevels, 0xCBB2},
		{"wEnemyMon", EnemyMon, 0xD0EF},
		{"wMenuCursorY", MenuCursorY, 0xCEE0},
		{"w2DMenuNumRows", TwoDMenuNumRows, 0xCEDA},
		{"wSaveFileExists", SaveFileExists, 0xD19A},
		{"wPlayerName", PlayerName, 0xD1A3},
		{"wRivalName", RivalName, 0xD1B9},
		{"wStatusFlags", StatusFlags, 0xD571},
		{"wElmsLabSceneID", ElmsLabSceneID, 0xD6CC},
		{"wRoute29SceneID", Route29SceneID, 0xD6CE},
		{"wCherrygroveCitySceneID", CherrygroveCitySceneID, 0xD6CF},
		{"wMrPokemonsHouseSceneID", MrPokemonsHouseSceneID, 0xD6D0},
		{"wEventFlags", EventFlags, 0xD7B7},
		{"wBattleResult", BattleResult, 0xCFE9},
		{"wNamingScreenDestinationPointer", NamingScreenDestination, 0xC5D0},
		{"wNamingScreenMaxNameLength", NamingScreenMaxNameLen, 0xC5D3},
		{"wInitHourBuffer", InitHourBuffer, 0xC51C},
		{"wInitMinuteBuffer", InitMinuteBuffer, 0xC526},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("%s = %#04x, want %#04x", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestOverworldBlockBufferMatchesRetailRange(t *testing.T) {
	if got := int(OverworldMap) + OverworldMapLen; got != 0xCC14 {
		t.Fatalf("wOverworldMapBlocks end = %#04x, want 0xcc14", got)
	}
}
