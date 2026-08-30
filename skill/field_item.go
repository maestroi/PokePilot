package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ErrFieldItemNoEffect reports that the field item's sequence ran to
// completion (menus closed, bag consumed) but the target mon's HP did not
// rise and its status byte was not cleared. A menu closing is not evidence
// of an effect; this is the positive postcondition failing.
var ErrFieldItemNoEffect = errors.New("skill: UseFieldItem: the item had no effect on the target")

// ErrFieldItemPrompt reports that a two-option (yes/no shaped) prompt was
// seen while paging the item's result text. It is returned WITHOUT pressing
// A into it: answering such a prompt by reflex has cost this project a
// caught Caterpie (S6-3) and a learned move (S6-4).
var ErrFieldItemPrompt = errors.New("skill: UseFieldItem: a two-option prompt appeared while paging the result text; not answering it")

// startMenuDrawBudget bounds the wait for DrawStartMenu to finish. The text
// box font loads before the menu is drawn, and wMaxMenuItem is the last
// write in DrawStartMenu, so it marks the menu as fully drawn (measured in
// TestSelectMenuItemStartMenu).
const startMenuDrawBudget = 60

// useTossBudget bounds the wait for the USE/TOSS two-option prompt that
// opens after A on a bag entry (start_sub_menus.asm .choseItem).
const useTossBudget = 60

// itemUsePartyBudget bounds the wait for the "Use item on which #MON?"
// party menu after A on the USE/TOSS prompt: the palette reload plus
// DrawPartyMenu's ClearScreen and mon-list redraw.
const itemUsePartyBudget = 1000

// fieldResultTextBudget bounds paging the result text closed: the
// "<NAME> recovered by NN!" box is a single prompt box, so this is generous
// on purpose; the cap fails loudly rather than hanging.
const fieldResultTextBudget = 3000

// itemListMenuID is wListMenuID for the bag list menu (ITEMLISTMENU). It is
// the positive identifier that the start menu's ITEM entry was selected:
// the party submenu (index 0) is not a list menu and leaves wListMenuID at
// 0 (measured in TestSelectMenuItemStartMenu).
const itemListMenuID = 3

// useTossPrompt reports the USE/TOSS two-option prompt SPECIFICALLY.
// DecodeTwoOptionMenu alone is not enough: the bag list itself presents as
// a two-option menu (one item row plus the CANCEL row — wMaxMenuItem=1,
// cursor on its own top tile, font loaded), and answering it by reflex
// selects the bag entry. The USE/TOSS box sits where start_sub_menus.asm
// .choseItem puts it: top item at (11,14).
func useTossPrompt(mem *state.Mem) *state.TwoOptionMenu {
	p := state.DecodeTwoOptionMenu(mem)
	if p == nil || mem.U8(sym.TopMenuItemY) != 11 || mem.U8(sym.TopMenuItemX) != 14 {
		return nil
	}
	return p
}

// startMenuShape reports the start menu's item count and the cursor index of
// its ITEM entry, derived from the same flag DrawStartMenu gates on
// (CheckEvent EVENT_GOT_POKEDEX, engine/menus/draw_start_menu.asm): without
// the pokedex the menu is POKEMON/ITEM/NAME/SAVE/OPTION/EXIT — six items,
// ITEM at index 1; with it, POKéDEX is printed FIRST (index 0) and every
// other entry shifts down one — seven items, ITEM at index 2. The flag is
// the source, not a constant: hardcoding either index breaks the moment the
// story crosses that flag.
func startMenuShape(mem *state.Mem) (max, itemIndex int) {
	max, itemIndex = 6, 1
	if state.HasEvent(mem, state.EventGotPokedex) {
		max, itemIndex = 7, 2
	}
	return max, itemIndex
}

// UseFieldItem uses one item from the bag on a party member from the
// overworld: START -> ITEM -> the item -> the party slot. The
// postcondition is POSITIVE and specific to the item's effect — for a heal,
// the target's current HP ROSE; for a status cure, the target's status byte
// CLEARED — read from state.DecodeParty before and after. A menu closing is
// not evidence.
//
// Every menu is step-and-verify: press, assert wCurrentMenuItem reached the
// wanted index, then A — SelectMenuItem for the start menu, selectBagEntry
// for the bag list, SelectPartySlot for the party menu; never a press count
// or a frame count. While paging the resulting text, a two-option prompt is
// STOPPED at and reported (ErrFieldItemPrompt), never answered.
func UseFieldItem(m *emu.Emu, item uint8, slot int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: UseFieldItem: player not controllable on map %#04x at (%d,%d): wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U16BE(sym.JoyIgnore), mem.U16BE(sym.FontLoaded))
	}
	party := state.DecodeParty(&mem)
	if slot < 0 || slot >= int(party.Count) {
		return fmt.Errorf("skill: UseFieldItem: slot %d out of range for a party of %d", slot, party.Count)
	}
	before := party.Mons[slot]
	idx, bagBefore := bagEntry(&mem, item)
	if idx < 0 {
		return fmt.Errorf("skill: UseFieldItem: %w (id %#02x)", ErrNotInBag, item)
	}

	// START menu. Its shape is derived from EVENT_GOT_POKEDEX (see
	// startMenuShape), and wMaxMenuItem finishing at the derived count is
	// the assertion that the menu is fully drawn in the shape we expect —
	// a stale or half-drawn menu fails here, not three menus later.
	wantMax, itemIndex := startMenuShape(&mem)
	drawn := func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0 && int(m.Peek8(sym.MaxMenuItem)) == wantMax
	}
	// The START edge is only processed while the overworld loop is running.
	// Right after a battle ends the game is still mid-return, and a single
	// Tap can be lost: measured, wFontLoaded stayed 0 for the whole draw
	// budget right after Battle returned, yet from the same state 120 frames
	// later the menu opened on the first tap. So tap repeatedly until drawn.
	// The menu does not close on START release, and StepUntil checks before
	// stepping, so a re-tap can never land on an already-open menu.
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := m.StepUntil(10, drawn); err == nil {
			break
		}
		m.Tap(emu.Start, 3, 7)
		if _, err := m.StepUntil(startMenuDrawBudget, drawn); err == nil {
			break
		}
	}
	state.Snapshot(m, &mem)
	if !drawn(m) {
		return fmt.Errorf("skill: UseFieldItem: start menu did not finish drawing: wFontLoaded=%#04x wCurrentMenuItem=%#04x wMaxMenuItem=%#04x (want max %d from pokedex flag)",
			mem.U8(sym.FontLoaded), mem.U8(sym.CurrentMenuItem), mem.U8(sym.MaxMenuItem), wantMax)
	}
	if err := SelectMenuItem(m, itemIndex); err != nil {
		return fmt.Errorf("skill: UseFieldItem: select ITEM (index %d): %w", itemIndex, err)
	}

	// Bag list: wListMenuID == ITEMLISTMENU is the positive identifier that
	// ITEM was selected (measured in TestSelectMenuItemStartMenu).
	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.ListMenuID) == itemListMenuID
	}); err != nil {
		state.Snapshot(m, &mem)
		return fmt.Errorf("skill: UseFieldItem: bag list did not open after ITEM: wFontLoaded=%#04x wListMenuID=%#04x",
			mem.U8(sym.FontLoaded), mem.U8(sym.ListMenuID))
	}
	// Selecting a bag entry opens the USE/TOSS prompt (start_sub_menus.asm
	// .choseItem), not the party menu directly. The A tap can land while
	// the list is still drawing and be lost — DisplayListMenuID sets hJoy7,
	// so only NEWLY pressed buttons count, and wListMenuID is written before
	// the box is drawn, so it is not a readiness signal. Step-and-verify:
	// tap, wait for the prompt specifically identified, re-tap if it was
	// lost (a lost tap moves nothing, so the cursor is still on idx).
	for attempt := 0; ; attempt++ {
		if err := selectBagEntry(m, idx); err != nil {
			return fmt.Errorf("skill: UseFieldItem: %w", err)
		}
		if _, err := m.StepUntil(useTossBudget, func(m *emu.Emu) bool {
			state.Snapshot(m, &mem)
			return useTossPrompt(&mem) != nil
		}); err == nil {
			break
		}
		if attempt >= 2 {
			state.Snapshot(m, &mem)
			return fmt.Errorf("skill: UseFieldItem: USE/TOSS prompt did not appear after selecting the bag entry (3 attempts): screen=%q wListMenuID=%#02x",
				state.ScreenText(&mem), mem.U8(sym.ListMenuID))
		}
	}
	// The prompt is answered with DecodeTwoOptionMenu — confirm it is up
	// and the cursor is on USE (index 0) before pressing A; never
	// frame-count into a choice.
	state.Snapshot(m, &mem)
	if p := useTossPrompt(&mem); p == nil || p.Index != 0 {
		return fmt.Errorf("skill: UseFieldItem: USE/TOSS cursor not on USE: screen=%q", state.ScreenText(&mem))
	}

	// "Use item on which #MON?" party menu (USE_ITEM_PARTY_MENU,
	// engine/items/item_effects.asm ItemUseMedicine). Same lost-tap
	// insurance: if the party menu does not appear and USE/TOSS is still
	// up, the A was dropped and is re-tapped.
	for attempt := 0; ; attempt++ {
		m.Tap(emu.A, 3, 7)
		if _, err := m.StepUntil(itemUsePartyBudget, func(m *emu.Emu) bool {
			return useItemPartyMenuUp(m)
		}); err == nil {
			break
		}
		state.Snapshot(m, &mem)
		if useTossPrompt(&mem) == nil || attempt >= 2 {
			return fmt.Errorf("skill: UseFieldItem: item-use party menu did not appear after USE: screen=%q wFontLoaded=%#02x",
				state.ScreenText(&mem), mem.U8(sym.FontLoaded))
		}
	}
	if err := SelectPartySlot(m, slot); err != nil {
		return fmt.Errorf("skill: UseFieldItem: %w", err)
	}

	// The result text ("recovered by NN!") and everything after it close with
	// B, not A. After the message (WaitForTextScrollButtonPress accepts A|B),
	// .done returns to start_sub_menus.asm .useItem_partyMenu, which jumps
	// back to the BAG LIST (jp StartMenu_Item) — measured: message ->
	// whiteout -> bag list -> start menu. B closes every link in that chain
	// and does nothing in the overworld, so press it until controllable. A
	// would be wrong here: on the bag list it selects an entry.
	state.Snapshot(m, &mem)
	if !(state.Controllable(&mem) && m.Peek8(sym.FontLoaded) == 0) {
		if _, err := m.StepUntil(500, func(m *emu.Emu) bool { return m.Peek8(sym.FontLoaded) != 0 }); err != nil {
			state.Snapshot(m, &mem)
			return fmt.Errorf("skill: UseFieldItem: result text did not appear within 500 frames: map=%#04x wFontLoaded=%#04x wJoyIgnore=%#04x",
				mem.U8(sym.CurMap), mem.U8(sym.FontLoaded), mem.U16BE(sym.JoyIgnore))
		}
	}
	start := m.FrameCount()
	for {
		state.Snapshot(m, &mem)
		if state.Controllable(&mem) && m.Peek8(sym.FontLoaded) == 0 {
			break
		}
		// Never answer a choice prompt blindly. The bag list itself decodes
		// as a two-option menu (one entry + CANCEL row) and B closes it;
		// the USE/TOSS prompt cannot be up at this point, but if it were,
		// B would TOSS — so stop and report.
		if p := useTossPrompt(&mem); p != nil {
			return fmt.Errorf("%w: cursor on option %d (map %#04x at (%d,%d))",
				ErrFieldItemPrompt, p.Index, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
		}
		if int(m.FrameCount()-start) > fieldResultTextBudget {
			state.Snapshot(m, &mem)
			return fmt.Errorf("skill: UseFieldItem: not back to the overworld after item use: screen=%q wFontLoaded=%#02x",
				state.ScreenText(&mem), mem.U8(sym.FontLoaded))
		}
		m.Tap(emu.B, 3, 7)
	}

	// Postcondition from RAM: the target's HP ROSE, or its status byte
	// CLEARED. A closed menu is not an effect.
	state.Snapshot(m, &mem)
	after := state.DecodeParty(&mem).Mons[slot]
	if !(after.HP > before.HP) && !(before.Status != 0 && after.Status == 0) {
		return fmt.Errorf("%w: slot %d HP %d/%d -> %d/%d, status %#02x -> %#02x (item %#02x)",
			ErrFieldItemNoEffect, slot, before.HP, before.MaxHP, after.HP, after.MaxHP,
			before.Status, after.Status, item)
	}
	if _, bagAfter := bagEntry(&mem, item); bagAfter != bagBefore-1 {
		return fmt.Errorf("skill: UseFieldItem: bag count for %#02x did not drop from %d (now %d)", item, bagBefore, bagAfter)
	}
	return nil
}
