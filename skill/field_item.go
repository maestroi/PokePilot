package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ErrFieldItemNoEffect reports that the field item's sequence ran to
// completion (menus closed, bag consumed) but the target mon did not gain HP,
// clear a status, or gain PP. A menu closing is not evidence of an effect;
// this is the positive postcondition failing.
var ErrFieldItemNoEffect = errors.New("skill: UseFieldItem: the item had no effect on the target")

// ErrFieldItemPrompt reports that a two-option (yes/no shaped) prompt was seen
// while paging the item's result text. It is returned WITHOUT pressing A into
// it: answering such a prompt by reflex has cost this project a caught
// Caterpie (S6-3) and a learned move (S6-4).
var ErrFieldItemPrompt = errors.New("skill: UseFieldItem: a two-option prompt appeared while paging the result text; not answering it")

const (
	startMenuDrawBudget = 60
	useTossBudget       = 60
	itemUsePartyBudget  = 1000
	itemUseMoveBudget   = 1000
	fieldResultTextBudget = 3000
	itemListMenuID      = 3

	// Gen 1 item IDs from constants/item_constants.asm. ETHER/MAX ETHER ask
	// for one move after the party member; ELIXER/MAX ELIXER restore every
	// move on the selected member immediately.
	itemEther     uint8 = 0x50
	itemMaxEther  uint8 = 0x51
	itemElixer    uint8 = 0x52
	itemMaxElixer uint8 = 0x53
)

// useTossPrompt reports the USE/TOSS two-option prompt SPECIFICALLY. The bag
// list itself can also decode as a two-option menu, so its screen position is
// part of the identity.
func useTossPrompt(mem *state.Mem) *state.TwoOptionMenu {
	p := state.DecodeTwoOptionMenu(mem)
	if p == nil || mem.U8(sym.TopMenuItemY) != 11 || mem.U8(sym.TopMenuItemX) != 14 {
		return nil
	}
	return p
}

// startMenuShape reports the start menu's item count and the cursor index of
// its ITEM entry, derived from EVENT_GOT_POKEDEX.
func startMenuShape(mem *state.Mem) (max, itemIndex int) {
	max, itemIndex = 6, 1
	if state.HasEvent(mem, state.EventGotPokedex) {
		max, itemIndex = 7, 2
	}
	return max, itemIndex
}

func isSingleMovePPRestore(item uint8) bool {
	return item == itemEther || item == itemMaxEther
}

func isPPRestoreItem(item uint8) bool {
	return item >= itemEther && item <= itemMaxElixer
}

// ppRestoreMoveSlot chooses the emptiest known move, preferring an exhausted
// move. Offer only proposes finite PP items at hard exhaustion, but keeping
// this helper deterministic makes direct skill callers safe too.
func ppRestoreMoveSlot(mon state.Mon) (int, bool) {
	best := -1
	var bestPP uint8
	for i, move := range mon.Moves {
		if move == 0 {
			continue
		}
		if mon.PP[i] == 0 {
			return i, true
		}
		if best < 0 || mon.PP[i] < bestPP {
			best, bestPP = i, mon.PP[i]
		}
	}
	return best, best >= 0
}

// fieldItemHadEffect is the positive postcondition shared by ordinary
// medicine and PP recovery. PP bytes are already decoded with PP-Up bits
// stripped by state.DecodeParty.
func fieldItemHadEffect(before, after state.Mon) bool {
	if after.HP > before.HP {
		return true
	}
	if before.Status != 0 && after.Status == 0 {
		return true
	}
	for i := range before.PP {
		if after.PP[i] > before.PP[i] {
			return true
		}
	}
	return false
}

// UseFieldItem uses one item from the bag on a party member from the
// overworld: START -> ITEM -> the item -> the party slot. Ether-style items
// additionally select a move when the ROM requests one. The postcondition is
// positive and item-effect based: HP rises, status clears, or PP rises; the
// bag count must also fall by exactly one.
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
	moveSlot := -1
	if isSingleMovePPRestore(item) {
		var ok bool
		moveSlot, ok = ppRestoreMoveSlot(before)
		if !ok {
			return fmt.Errorf("skill: UseFieldItem: PP restore target slot %d has no known moves", slot)
		}
	}
	idx, bagBefore := bagEntry(&mem, item)
	if idx < 0 {
		return fmt.Errorf("skill: UseFieldItem: %w (id %#02x)", ErrNotInBag, item)
	}

	wantMax, itemIndex := startMenuShape(&mem)
	drawn := func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0 && int(m.Peek8(sym.MaxMenuItem)) == wantMax
	}
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

	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.ListMenuID) == itemListMenuID
	}); err != nil {
		state.Snapshot(m, &mem)
		return fmt.Errorf("skill: UseFieldItem: bag list did not open after ITEM: wFontLoaded=%#04x wListMenuID=%#04x",
			mem.U8(sym.FontLoaded), mem.U8(sym.ListMenuID))
	}
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
	state.Snapshot(m, &mem)
	if p := useTossPrompt(&mem); p == nil || p.Index != 0 {
		return fmt.Errorf("skill: UseFieldItem: USE/TOSS cursor not on USE: screen=%q", state.ScreenText(&mem))
	}

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

	// ETHER and MAX ETHER run MoveSelectionMenu after the party choice. The
	// ROM explicitly sets wMoveMenuType=2 for this field menu; MoveSelectionMenu
	// then uses 1-based wCurrentMenuItem, exactly like battle move selection.
	if isSingleMovePPRestore(item) {
		if _, err := m.StepUntil(itemUseMoveBudget, func(m *emu.Emu) bool {
			return m.Peek8(sym.MoveMenuType) == 2 && m.Peek8(sym.CurrentMenuItem) >= 1
		}); err != nil {
			state.Snapshot(m, &mem)
			return fmt.Errorf("skill: UseFieldItem: PP move menu did not appear: item=%#02x slot=%d screen=%q wMoveMenuType=%#02x cursor=%d",
				item, slot, state.ScreenText(&mem), mem.U8(sym.MoveMenuType), mem.U8(sym.CurrentMenuItem))
		}
		if err := SelectMenuItem(m, moveSlot+1); err != nil {
			return fmt.Errorf("skill: UseFieldItem: select move slot %d for PP restore: %w", moveSlot, err)
		}
	}

	// The result text and everything after it close with B, not A. B closes
	// the result, bag list and start menu while doing nothing in the overworld.
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

	state.Snapshot(m, &mem)
	afterParty := state.DecodeParty(&mem)
	if slot >= len(afterParty.Mons) {
		return fmt.Errorf("skill: UseFieldItem: party slot %d disappeared after item use", slot)
	}
	after := afterParty.Mons[slot]
	if !fieldItemHadEffect(before, after) {
		return fmt.Errorf("%w: slot %d HP %d/%d -> %d/%d, status %#02x -> %#02x, PP %v -> %v (item %#02x, ppRestore=%v)",
			ErrFieldItemNoEffect, slot, before.HP, before.MaxHP, after.HP, after.MaxHP,
			before.Status, after.Status, before.PP, after.PP, item, isPPRestoreItem(item))
	}
	if _, bagAfter := bagEntry(&mem, item); bagAfter != bagBefore-1 {
		return fmt.Errorf("skill: UseFieldItem: bag count for %#02x did not drop from %d (now %d)", item, bagBefore, bagAfter)
	}
	return nil
}
