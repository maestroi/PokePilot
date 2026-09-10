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
// list-backed menus and is retained for diagnostics without requiring callers
// to interpret it just to distinguish the common semantic families.
type InteractionState struct {
	Kind       InteractionKind
	Text       string
	Current    int
	Max        int
	ListMenuID uint8
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
	if !MenuUp(m) || m.U8(sym.MenuWatchedKeys) != listMenuWatchedKeys {
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
