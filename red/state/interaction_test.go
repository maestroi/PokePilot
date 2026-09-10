package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func interactionMenuFixture(max, cur, listID, watched byte) *Mem {
	m := twoOptionFixture(1, 0, max, cur, 8, 12, menuCursorTile)
	m[sym.ListMenuID] = listID
	m[sym.MenuWatchedKeys] = watched
	return m
}

func TestDecodeInteractionDialogue(t *testing.T) {
	var m Mem
	m[sym.FontLoaded] = 1
	if got := DecodeInteraction(&m); got.Kind != InteractionDialogue {
		t.Fatalf("DecodeInteraction kind = %q, want dialogue", got.Kind)
	}
}

func TestDecodeInteractionTwoOption(t *testing.T) {
	m := twoOptionFixture(1, 0, 1, 1, 8, 12, menuCursorTile)
	m[sym.TwoOptionMenuID] = yesNoMenuID
	got := DecodeInteraction(m)
	if got.Kind != InteractionTwoOption || got.Current != 1 || got.Max != 1 {
		t.Fatalf("DecodeInteraction = %+v, want live two-option index 1", got)
	}
	if got.Options != [2]string{"YES", "NO"} {
		t.Fatalf("two-option labels = %q, want YES/NO", got.Options)
	}
}

func TestDecodeInteractionNoYesOrdering(t *testing.T) {
	m := twoOptionFixture(1, 0, 1, 0, 8, 12, menuCursorTile)
	m[sym.TwoOptionMenuID] = noYesMenuID | 0x80 // second-option-default bit
	got := DecodeInteraction(m)
	if got.Options != [2]string{"NO", "YES"} {
		t.Fatalf("NO/YES labels = %q, want NO/YES", got.Options)
	}
}

func TestTwoOptionLabelsRejectUnknownLayout(t *testing.T) {
	if got := twoOptionLabels(0x7f); got != [2]string{} {
		t.Fatalf("unknown two-option labels = %q, want empty labels", got)
	}
}

func TestDecodeInteractionShortItemListIsNotChoice(t *testing.T) {
	m := interactionMenuFixture(1, 0, itemListMenuID, listMenuWatchedKeys)
	got := DecodeInteraction(m)
	if got.Kind != InteractionItemMenu {
		t.Fatalf("DecodeInteraction kind = %q, want item_menu for a live short bag list", got.Kind)
	}
}

func TestDecodeInteractionListUsesScrollOffset(t *testing.T) {
	m := interactionMenuFixture(2, 2, pricedItemListMenuID, listMenuWatchedKeys)
	m[sym.ListScrollOffset] = 3
	got := DecodeInteraction(m)
	if got.Kind != InteractionItemMenu || got.Current != 5 {
		t.Fatalf("DecodeInteraction = %+v, want item_menu at absolute list index 5", got)
	}
}

func TestDecodeInteractionStaleListIDDoesNotHideChoice(t *testing.T) {
	m := twoOptionFixture(1, 0, 1, 0, 8, 12, menuCursorTile)
	m[sym.ListMenuID] = itemListMenuID // stale after closing the bag
	m[sym.MenuWatchedKeys] = 3         // live DisplayTwoOptionMenu controller
	got := DecodeInteraction(m)
	if got.Kind != InteractionTwoOption {
		t.Fatalf("DecodeInteraction kind = %q, want two_option despite stale list id", got.Kind)
	}
}

func TestClassifySpecialListElevator(t *testing.T) {
	if got := classifyListInteraction(specialListMenuID, "Which floor do you want?"); got != InteractionElevatorMenu {
		t.Fatalf("special floor list = %q, want elevator_menu", got)
	}
	if got := classifyListInteraction(specialListMenuID, "BADGES"); got != InteractionListMenu {
		t.Fatalf("non-elevator special list = %q, want list_menu", got)
	}
}

func TestClassifyCursorMenus(t *testing.T) {
	cases := []struct {
		name string
		text string
		want InteractionKind
	}{
		{"pc root", "BILL'S PC LOG OFF", InteractionPCMenu},
		{"bills pc", "WITHDRAW DEPOSIT RELEASE CHANGE BOX", InteractionPCMenu},
		{"party", "Choose a Pokemon.", InteractionPartyMenu},
		{"generic", "BUY SELL QUIT", InteractionMenu},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyCursorMenu(tc.text); got != tc.want {
				t.Fatalf("classifyCursorMenu(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}
