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
		{"wLastPocket", LastPocket, 0xCFC8},
		{"wTMHMPocketCursor", TMHMPocketCursor, 0xCFCD},
		{"wTMHMPocketScrollPosition", TMHMPocketScroll, 0xCFD2},
		{"wNumMoves", NumMoves, 0xCFE3},
		{"wBattlePlayerAction", BattlePlayerAction, 0xCFE4},
		{"wFieldMoveSucceeded", FieldMoveSucceeded, 0xCFE4},
		{"wCurPartyMon", CurPartyMon, 0xD005},
		{"wPutativeTMHMMove", PutativeTMHMMove, 0xD14D},
		{"wTextboxFlags", TextboxFlags, 0xD19C},
		{"wTileMap", TileMap, 0xC3A0},
		{"wBattleMon", BattleMon, 0xCB0C},
		{"wPlayerDisableCount", PlayerDisableCount, 0xCB53},
		{"wPlayerStatLevels", PlayerStatLevels, 0xCBAA},
		{"wEnemyStatLevels", EnemyStatLevels, 0xCBB2},
		{"wPlayerScreens", PlayerScreens, 0xCBDD},
		{"wEnemyMon", EnemyMon, 0xD0EF},
		{"wMenuCursorY", MenuCursorY, 0xCEE0},
		{"w2DMenuNumRows", TwoDMenuNumRows, 0xCEDA},
		{"wJumptableIndex", JumptableIndex, 0xCE63},
		{"wPackJumptableIndex", PackJumptableIndex, 0xCE64},
		{"wCurPocket", CurPocket, 0xCE65},
		{"wPackUsedItem", PackUsedItem, 0xCE66},
		{"wSaveFileExists", SaveFileExists, 0xD19A},
		{"wPlayerName", PlayerName, 0xD1A3},
		{"wRivalName", RivalName, 0xD1B9},
		{"wTimeOfDayPalset", TimeOfDayPalset, 0xD56E},
		{"wStatusFlags", StatusFlags, 0xD571},
		{"wStatusFlags2", StatusFlags2, 0xD572},
		{"wPokegearFlags", PokegearFlags, 0xD67C},
		{"wElmsLabSceneID", ElmsLabSceneID, 0xD6CC},
		{"wRoute29SceneID", Route29SceneID, 0xD6CE},
		{"wCherrygroveCitySceneID", CherrygroveCitySceneID, 0xD6CF},
		{"wMrPokemonsHouseSceneID", MrPokemonsHouseSceneID, 0xD6D0},
		{"wEventFlags", EventFlags, 0xD7B7},
		{"wBikeFlags", BikeFlags, 0xD93F},
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

// TestGoldPackUnionArmOffsetsFollowWram pins the pack arm of the wJumptableIndex
// union against ram/wram.asm, which declares wPackJumptableIndex, wCurPocket and
// wPackUsedItem as consecutive NEXTU fields starting one byte after
// wJumptableIndex. Reading wCurPocket at the title arm's offset reported the
// pack's jumptable state as the pocket, so the items pocket decoded as "balls"
// and the TM/HM pocket as unknown, which made every Gen-II field-move teach fail
// with "machine menu could not reach TM/HM pocket".
func TestGoldPackUnionArmOffsetsFollowWram(t *testing.T) {
	if PackJumptableIndex != JumptableIndex+1 {
		t.Fatalf("wPackJumptableIndex = %#04x, want wJumptableIndex+1 = %#04x", PackJumptableIndex, JumptableIndex+1)
	}
	if CurPocket != PackJumptableIndex+1 {
		t.Fatalf("wCurPocket = %#04x, want wPackJumptableIndex+1 = %#04x", CurPocket, PackJumptableIndex+1)
	}
	if PackUsedItem != CurPocket+1 {
		t.Fatalf("wPackUsedItem = %#04x, want wCurPocket+1 = %#04x", PackUsedItem, CurPocket+1)
	}
	// The title arm shares the union base but not the pack fields.
	if TitleScreenSelected != PackJumptableIndex {
		t.Fatalf("wTitleScreenSelectedOption = %#04x, want %#04x", TitleScreenSelected, PackJumptableIndex)
	}
	if TitleScreenTimer != CurPocket {
		t.Fatalf("wTitleScreenTimer = %#04x, want %#04x", TitleScreenTimer, CurPocket)
	}
}
