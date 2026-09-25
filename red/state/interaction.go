package state

import (
	"strings"

	"github.com/maestroi/pokepilot/red/sym"
)

// InteractionKind describes the semantic UI surface currently accepting
// player input. It deliberately stays below story semantics: callers can
// distinguish dialogue, choices and menu families without guessing from raw
// cursor bytes, but still own the meaning of a particular story question.
type InteractionKind string

const (
	InteractionNone          InteractionKind = "none"
	InteractionDialogue      InteractionKind = "dialogue"
	InteractionTwoOption     InteractionKind = "two_option"
	InteractionMenu          InteractionKind = "menu"
	InteractionListMenu      InteractionKind = "list_menu"
	InteractionElevatorMenu  InteractionKind = "elevator_menu"
	InteractionItemMenu      InteractionKind = "item_menu"
	InteractionPartyMenu     InteractionKind = "party_menu"
	InteractionPCMenu        InteractionKind = "pc_menu"
	InteractionPCPokemonList InteractionKind = "pc_pokemon_list"
)

// Gen I list menu IDs from pokered/constants/list_constants.asm.
const (
	pcPokemonListMenuID  = 0
	movesListMenuID      = 1
	pricedItemListMenuID = 2
	itemListMenuID       = 3
	specialListMenuID    = 4
)

// DisplayListMenuID watches A|B|SELECT. Using that live controller state is
// important because wListMenuID is stale after a list closes; the ID alone
// must never turn a later two-option prompt into an item/list menu.
const listMenuWatchedKeys = 7

// InteractionState is a compact, positively decoded view of the active UI.
// Current/Max are meaningful for menu kinds. ListMenuID is meaningful for
// list-backed menus. Options contains the ordered semantic labels for a live
// two-option prompt, so callers can ask for YES rather than assuming it is
// always menu index 0.
type InteractionState struct {
	Kind       InteractionKind
	Text       string
	Current    int
	Max        int
	ListMenuID uint8
	Options    [2]string
}

// DecodeInteraction returns the active typed interaction surface. Unknown
// cursor menus fail closed to InteractionMenu/ListMenu rather than being
// guessed into a story-specific type.
func DecodeInteraction(m *Mem) InteractionState {
	text := ScreenText(m)

	// A short scrolling list can have the same wMaxMenuItem==1 shape as a
	// two-option prompt. Detect a live DisplayListMenuID surface first; unlike
	// wListMenuID, wMenuWatchedKeys is written by the live list controller.
	if liveListMenu(m) {
		menu := DecodeMenu(m)
		id := m.U8(sym.ListMenuID)
		return InteractionState{
			Kind:       classifyListInteraction(id, text),
			Text:       text,
			Current:    int(m.U8(sym.ListScrollOffset)) + menu.Current,
			Max:        menu.Max,
			ListMenuID: id,
		}
	}

	if prompt := DecodeTwoOptionMenu(m); prompt != nil {
		return InteractionState{
			Kind:    InteractionTwoOption,
			Text:    text,
			Current: prompt.Index,
			Max:     1,
			Options: twoOptionLabels(m),
		}
	}

	if MenuUp(m) {
		menu := DecodeMenu(m)
		return InteractionState{
			Kind:    classifyCursorMenu(text),
			Text:    text,
			Current: menu.Current,
			Max:     menu.Max,
		}
	}

	if dialogue := DecodeDialogue(m); dialogue != nil {
		return InteractionState{Kind: InteractionDialogue, Text: dialogue.Text}
	}
	return InteractionState{Kind: InteractionNone}
}

func liveListMenu(m *Mem) bool {
	// wFontLoaded stays 0 for a whole battle (see DecodeTwoOptionMenu), so the
	// battle bag's drawn-text gate is wIsInBattle; the cursor glyph still
	// rejects stale list RAM.
	if m.U8(sym.FontLoaded) == 0 && m.U8(sym.IsInBattle) == 0 || !menuCursorDrawn(m) ||
		m.U8(sym.MenuWatchedKeys) != listMenuWatchedKeys {
		return false
	}
	return m.U8(sym.ListMenuID) <= specialListMenuID
}

func classifyListInteraction(id uint8, text string) InteractionKind {
	switch id {
	case pcPokemonListMenuID:
		return InteractionPCPokemonList
	case pricedItemListMenuID, itemListMenuID:
		return InteractionItemMenu
	case specialListMenuID:
		if strings.Contains(strings.ToLower(text), "which floor") {
			return InteractionElevatorMenu
		}
		return InteractionListMenu
	case movesListMenuID:
		return InteractionListMenu
	default:
		return InteractionListMenu
	}
}

func classifyCursorMenu(text string) InteractionKind {
	upper := strings.ToUpper(text)
	if strings.Contains(upper, "WITHDRAW") && strings.Contains(upper, "DEPOSIT") ||
		strings.Contains(upper, "LOG OFF") && strings.Contains(upper, "PC") {
		return InteractionPCMenu
	}
	if strings.Contains(upper, "CHOOSE") {
		return InteractionPartyMenu
	}
	return InteractionMenu
}

// twoOptionLabels reads the two labels DisplayTwoOptionMenu drew right of the
// cursor column, one PlaceString <NEXT> (two rows) apart. wTwoOptionMenuID
// cannot name them: the ROM zeroes it before HandleMenuInput for every menu
// (text_box.asm .notNoYesMenu and the NO/YES branch), so a live TRADE/CANCEL
// or HEAL/CANCEL menu read back as YES/NO.
func twoOptionLabels(m *Mem) [2]string {
	const screenWidth, screenHeight = 20, 18 // wTileMap geometry
	x, y := int(m.U8(sym.TopMenuItemX)), int(m.U8(sym.TopMenuItemY))
	if x+1 >= screenWidth || y+2 >= screenHeight {
		return [2]string{}
	}
	tiles := m.Slice(sym.TileMap, sym.TileMapLen)
	var out [2]string
	for i := range out {
		row := (y + 2*i) * screenWidth
		fields := strings.Fields(DecodeTiles(tiles[row+x+1 : row+screenWidth]))
		if len(fields) == 0 {
			return [2]string{}
		}
		out[i] = strings.ToUpper(fields[0])
	}
	return out
}
