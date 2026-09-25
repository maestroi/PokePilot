package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func (*Profile) DecodeBattleRuntime(reader game.MemoryReader) game.BattleRuntimeState {
	if reader == nil {
		return game.BattleRuntimeState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	menu := state.DecodeMenu(&mem)
	facts := state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem))
	return game.BattleRuntimeState{
		InBattle:         state.DecodeBattle(&mem) != nil,
		Controllable:     state.Controllable(&mem),
		TextActive:       mem.U8(sym.FontLoaded) != 0,
		CampaignComplete: facts.LeagueChampionDefeated,
		NativeMapID:      uint16(mem.U8(sym.CurMap)),
		X:                mem.U8(sym.XCoord),
		Y:                mem.U8(sym.YCoord),
		DebugText:        state.ScreenText(&mem),
		MenuCursor: game.MenuCursorState{
			Current: menu.Current,
			Max:     menu.Max,
		},
	}
}
