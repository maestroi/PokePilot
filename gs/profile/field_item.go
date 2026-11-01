package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

// Gen-II native item IDs (pret/pokegold constants/item_constants.asm). These
// are deliberately not the similarly named Gen-I IDs in skill/battle_resources.
const (
	gsMedicineAntidote    uint16 = 0x09
	gsMedicineBurnHeal    uint16 = 0x0a
	gsMedicineIceHeal     uint16 = 0x0b
	gsMedicineAwakening   uint16 = 0x0c
	gsMedicineParlyzHeal  uint16 = 0x0d
	gsMedicineFullRestore uint16 = 0x0e
	gsMedicineMaxPotion   uint16 = 0x0f
	gsMedicineHyperPotion uint16 = 0x10
	gsMedicineSuperPotion uint16 = 0x11
	gsMedicinePotion      uint16 = 0x12
	gsMedicineMaxElixer   uint16 = 0x15
	gsMedicineFullHeal    uint16 = 0x26
	gsMedicineRevive      uint16 = 0x27
	gsMedicineMaxRevive   uint16 = 0x28
	gsMedicineFreshWater  uint16 = 0x2e
	gsMedicineSodaPop     uint16 = 0x2f
	gsMedicineLemonade    uint16 = 0x30
	gsMedicineEther       uint16 = 0x3f
	gsMedicineMaxEther    uint16 = 0x40
	gsMedicineElixer      uint16 = 0x41
)

var _ game.FieldItemDecoder = (*Profile)(nil)

func (*Profile) FieldItemSemantics(item uint16) game.FieldItemSemantics {
	switch item {
	case gsMedicineEther, gsMedicineMaxEther:
		return game.FieldItemSemantics{PPRestore: true, SingleMoveTarget: true}
	case gsMedicineElixer, gsMedicineMaxElixer:
		return game.FieldItemSemantics{PPRestore: true}
	default:
		return game.FieldItemSemantics{}
	}
}

// Repel is intentionally not advertised to the generic field-item skill
// until Gold/Silver's repel counter/menu sequence has been qualified.
func (*Profile) PreferredRepels() []uint16 { return nil }

// DecodeFieldItem reports *rendered* Gen-II menu states. Shared menu RAM
// persists after the PACK closes; it must never be treated as live without
// matching the screen. Current HP/PP is read from the actual party structs.
func (p *Profile) DecodeFieldItem(reader game.MemoryReader) game.FieldItemState {
	if reader == nil {
		return game.FieldItemState{}
	}
	text := gsScreenText(reader)
	count := min(int(reader.Peek8(sym.PartyCount)), 6)
	out := game.FieldItemState{
		Party: make([]game.FieldItemPartyMon, 0, count),
		InBattle: reader.Peek8(sym.BattleMode) != 0,
		DebugText: text,
	}
	for i := 0; i < count; i++ {
		base := sym.PartyMon1 + uint16(i)*sym.PartyMonSize
		mon := game.FieldItemPartyMon{
			NativeSpeciesID: uint16(reader.Peek8(base)),
			Level: reader.Peek8(base + gen2PartyLevelOffset),
			HP: battleBE16(reader, base+gen2PartyHPOffset),
			MaxHP: battleBE16(reader, base+gen2PartyMaxHPOffset),
			Status: gen2Status(reader.Peek8(base+gen2PartyStatusOffset)),
		}
		for move := 0; move < 4; move++ {
			mon.Moves[move] = uint16(reader.Peek8(base+gen2PartyMovesOffset+uint16(move)))
			mon.PP[move] = reader.Peek8(base+gen2PartyPPOffset+uint16(move)) & gen2PPMask
		}
		out.Party = append(out.Party, mon)
	}
	y, x, rows, cols, _ := gsMenuCursor(reader)
	pack := gsPackVisible(reader)
	start := p.DecodeStartMenu(reader).Visible
	// The native 4-row USE/GIVE/TOSS/QUIT box overwrites the left-hand
	// PACK graphic, so unlike the scrolling pocket it cannot depend on
	// gsPackVisible. Require the whole rendered menu instead of stale RAM.
	out.UsePromptVisible = !out.InBattle &&
		strings.Contains(text, "USE") && strings.Contains(text, "GIVE") &&
		strings.Contains(text, "TOSS") && strings.Contains(text, "QUIT") &&
		reader.Peek8(sym.CurPocket) == gen2PocketItems &&
		cols == 1 && rows == 4 && x == 1 && y >= 1 && y <= rows
	out.UseSelected = out.UsePromptVisible && y == 1
	out.MoveMenuVisible = !out.InBattle &&
		strings.Contains(strings.ToLower(text), "which move?") &&
		cols == 1 && rows >= 1 && rows <= 4 && x == 1 && y >= 1 && y <= rows
	out.MoveCursor = game.MenuCursorState{Current: max(0, int(y)-1), Max: max(0, int(rows)-1)}
	_, out.ChoiceVisible = p.DecodeTwoOption(reader)
	out.ResultTextActive = gsTextboxVisible(reader) && !out.ChoiceVisible
	out.UIOpen = pack || start || out.UsePromptVisible || out.MoveMenuVisible ||
		p.DecodePartyMenu(reader).Visible || out.ResultTextActive || out.ChoiceVisible
	out.OverworldReady = gsControllable(reader) && !out.InBattle && !out.UIOpen
	return out
}
