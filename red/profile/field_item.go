package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	itemRepel      uint16 = 0x1e
	itemSuperRepel uint16 = 0x38
	itemMaxRepel   uint16 = 0x39
	itemEther      uint16 = 0x50
	itemMaxEther   uint16 = 0x51
	itemElixer     uint16 = 0x52
	itemMaxElixer  uint16 = 0x53
)

func (*Profile) FieldItemSemantics(item uint16) game.FieldItemSemantics {
	switch item {
	case itemRepel:
		return game.FieldItemSemantics{RepelSteps: 100}
	case itemSuperRepel:
		return game.FieldItemSemantics{RepelSteps: 200}
	case itemMaxRepel:
		return game.FieldItemSemantics{RepelSteps: 250}
	case itemEther, itemMaxEther:
		return game.FieldItemSemantics{SingleMoveTarget: true, PPRestore: true}
	case itemElixer, itemMaxElixer:
		return game.FieldItemSemantics{PPRestore: true}
	default:
		return game.FieldItemSemantics{}
	}
}

func (*Profile) PreferredRepels() []uint16 {
	return []uint16{itemMaxRepel, itemSuperRepel, itemRepel}
}

func (*Profile) DecodeFieldItem(reader game.MemoryReader) game.FieldItemState {
	if reader == nil {
		return game.FieldItemState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	party := state.DecodeParty(&mem)
	out := game.FieldItemState{
		Party:      make([]game.FieldItemPartyMon, 0, len(party.Mons)),
		InBattle:   state.DecodeBattle(&mem) != nil,
		RepelSteps: int(mem.U8(sym.RepelRemainingSteps)),
		DebugText:  state.ScreenText(&mem),
	}
	for _, mon := range party.Mons {
		p := game.FieldItemPartyMon{
			NativeSpeciesID: uint16(mon.Species),
			Level:           mon.Level, HP: mon.HP, MaxHP: mon.MaxHP,
			Status: mon.StatusName(),
		}
		for i := range mon.Moves {
			p.Moves[i] = uint16(mon.Moves[i])
			p.PP[i] = mon.PP[i]
		}
		out.Party = append(out.Party, p)
	}
	prompt := state.DecodeTwoOptionMenu(&mem)
	out.ChoiceVisible = prompt != nil
	if prompt != nil && mem.U8(sym.TopMenuItemY) == 11 && mem.U8(sym.TopMenuItemX) == 14 {
		out.UsePromptVisible = true
		out.UseSelected = prompt.Index == 0
	}
	if mem.U8(sym.MoveMenuType) == 2 && mem.U8(sym.CurrentMenuItem) >= 1 {
		out.MoveMenuVisible = true
		out.MoveCursor = game.MenuCursorState{
			Current: int(mem.U8(sym.CurrentMenuItem)) - 1,
			Max:     maxKnownMoveSlot(party),
		}
	}
	interaction := state.DecodeInteraction(&mem)
	out.ResultTextActive = mem.U8(sym.FontLoaded) != 0 || state.DecodeDialogue(&mem) != nil
	out.UIOpen = interaction.Kind != state.InteractionNone || state.MenuUp(&mem) || out.ResultTextActive
	out.OverworldReady = state.Controllable(&mem) && interaction.Kind == state.InteractionNone && !state.MenuUp(&mem) && mem.U8(sym.FontLoaded) == 0
	return out
}

func maxKnownMoveSlot(party state.PartyState) int {
	max := 0
	for _, mon := range party.Mons {
		for i, move := range mon.Moves {
			if move != 0 && i > max {
				max = i
			}
		}
	}
	return max
}
