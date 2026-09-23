package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
)

func TestGenericTalkDeclinableYesNoAcceptsBothLayouts(t *testing.T) {
	cases := []state.InteractionState{
		{Kind: state.InteractionTwoOption, Options: [2]string{"YES", "NO"}},
		{Kind: state.InteractionTwoOption, Options: [2]string{"NO", "YES"}},
	}
	for _, interaction := range cases {
		if !genericTalkDeclinableYesNo(interaction) {
			t.Fatalf("expected YES/NO interaction to be safely declinable: %+v", interaction)
		}
	}
}

func TestGenericTalkDeclinableYesNoRejectsOtherInteractions(t *testing.T) {
	cases := []state.InteractionState{
		{Kind: state.InteractionTwoOption, Options: [2]string{"TRADE", "CANCEL"}},
		{Kind: state.InteractionTwoOption, Options: [2]string{"HEAL", "CANCEL"}},
		{Kind: state.InteractionTwoOption, Options: [2]string{"NORTH", "WEST"}},
		{Kind: state.InteractionMenu, Options: [2]string{"YES", "NO"}},
		{Kind: state.InteractionDialogue},
	}
	for _, interaction := range cases {
		if genericTalkDeclinableYesNo(interaction) {
			t.Fatalf("unexpected interaction was classified as safely declinable: %+v", interaction)
		}
	}
}

// TestDismissableListMenuIsRoutedToCancelNotYesNoDecline pins the Cerulean
// BadgeHouse regression: "Which of the 8 BADGEs should I describe?" is a
// SPECIALLISTMENU (pokered/scripts/CeruleanBadgeHouse.asm), not a YES/NO
// prompt. Before this fix, declineUnexpectedGenericTalkChoice only recognized
// genericTalkDeclinableYesNo interactions and left this list menu's
// ErrTalkMenu unhandled. skill.DismissableObjectiveMenu must classify it as
// cancellable so declineUnexpectedGenericTalkChoice's menu branch fires.
func TestDismissableListMenuIsRoutedToCancelNotYesNoDecline(t *testing.T) {
	var mem state.Mem
	mem[sym.FontLoaded] = 1
	mem[sym.MenuWatchedKeys] = 7 // live DisplayListMenuID controller
	mem[sym.ListMenuID] = 4      // SPECIALLISTMENU
	mem[sym.TopMenuItemY] = 8
	mem[sym.TopMenuItemX] = 12
	// The cursor is read from wMenuCursorLocation, so record where the glyph
	// was drawn the way PlaceMenuCursor does; the top-item coordinates alone
	// are only the first entry's position.
	offset := uint16(8*20 + 12)
	mem[sym.TileMap+offset] = 0xED // cursor glyph PlaceMenuCursor draws
	cursor := sym.TileMap + offset
	mem[sym.MenuCursorLocation] = byte(cursor)
	mem[sym.MenuCursorLocation+1] = byte(cursor >> 8)

	interaction := state.DecodeInteraction(&mem)
	if interaction.Kind != state.InteractionListMenu {
		t.Fatalf("DecodeInteraction kind = %q, want list_menu", interaction.Kind)
	}
	if genericTalkDeclinableYesNo(interaction) {
		t.Fatal("a list menu must not be treated as a YES/NO prompt")
	}
	if !skill.DismissableObjectiveMenu(&mem) {
		t.Fatal("an unclassified list menu must still be a safe cancel target for declineUnexpectedGenericTalkChoice")
	}
}
