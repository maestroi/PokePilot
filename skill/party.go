package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// partyMenuMarker identifies the FORCED battle party menu from wTileMap.
// It is the footer line _PartyMenuBattleText ("Bring out which #MON?") that
// DrawPartyMenu prints for BATTLE_PARTY_MENU (engine/menus/party_menu.asm);
// the other party menu types print different footers, and no other battle
// screen contains this line. As with every battle screen it comes from
// wTileMap, never wFontLoaded, which stays 0 for the whole of a battle.
const partyMenuMarker = "Bring out"

// partyMenuUp reports whether the forced (after-a-faint) battle party menu
// is on screen.
func partyMenuUp(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return battleScreenHas(m, partyMenuMarker)
}

// useItemPartyMenuMarker identifies the OVERWORLD item-use party menu from
// wTileMap: the footer line _PartyMenuItemUseText ("Use item on which
// #MON?") that DrawPartyMenu prints for USE_ITEM_PARTY_MENU
// (engine/items/item_effects.asm ItemUseMedicine); no battle screen and no
// other overworld menu contains this line.
const useItemPartyMenuMarker = "Use item"

// useItemPartyMenuUp reports whether the overworld item-use party menu is on
// screen.
func useItemPartyMenuUp(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return battleScreenHas(m, useItemPartyMenuMarker)
}

// The FIGHT/ITEM/PKMN/RUN battle menu is a 2x2 grid: FIGHT/ITEM in the left
// column, PKMN/RUN in the right (DisplayBattleMenu, engine/battle/core.asm).
// The cursor's tile X sits in wTopMenuItemX: $9 for the left column, $f for
// the right; wCurrentMenuItem holds the row (0 is the top). A press in the
// right column adds $2 to the row before dispatch, so POKéMON selects as
// item 2 — unreachable by SelectMenuItem, whose range check reads the
// per-column wMaxMenuItem of 1.
const (
	battleMenuLeftX  byte = 0x09
	battleMenuRightX byte = 0x0F
)

// SetLead reorders the party through the start menu's POKEMON list so that
// the member currently in slot is slot 0, the lead. A party of one needs no
// decisions: SetLead(m, 0) verifies the range and returns without touching
// a button.
//
// The game's only reorder is a two-way swap between the lead and the
// selected member (SwitchPartyMon), so any wanted slot reaches the front in
// one swap — which is exactly what PromoteToLead drives, step-and-verify,
// from START -> PKMN to the overworld again. POSITIVE postcondition:
// state.DecodeParty's first member is the species that sat in slot before
// the call; PromoteToLead returns an error when it does not hold.
func SetLead(m *emu.Emu, slot int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	if slot < 0 || slot >= int(party.Count) {
		return fmt.Errorf("skill: SetLead: slot %d out of range for a party of %d", slot, party.Count)
	}
	if slot == 0 {
		return nil // already the lead; nothing to decide
	}
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: SetLead: player not controllable on map %#04x", m.Peek8(sym.CurMap))
	}
	return PromoteToLead(m, slot)
}

// SwitchActive performs the VOLUNTARY mid-battle switch through the battle
// menu's POKéMON branch: A opens the FIGHT/ITEM/PKMN/RUN menu, RIGHT moves
// the cursor to the right column (POKéMON), A opens the party menu, the
// wanted slot is selected, and the SWITCH/STATS/CANCEL box that follows is
// answered SWITCH. POSITIVE postcondition: state.DecodeBattle's ActiveSpecies
// is the species that sat in party slot before the call — a nil return with
// the same active mon would be a lie, so the species is re-read from RAM at
// the end and asserted.
//
// This is the half of the battle party menu that Battle does not drive:
// the forced switch after a faint (S6-5b) is answered inside Battle's state
// machine; this one is opened by the player. Every step is press, assert,
// A — the column is asserted from wTopMenuItemX, the row and the party slot
// from wCurrentMenuItem, each menu from its own wTileMap marker.
func SwitchActive(m *emu.Emu, slot int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) == nil {
		return fmt.Errorf("skill: SwitchActive: no battle in progress on map %#04x", m.Peek8(sym.CurMap))
	}
	party := state.DecodeParty(&mem)
	if slot < 0 || slot >= int(party.Count) {
		return fmt.Errorf("skill: SwitchActive: slot %d out of range for a party of %d", slot, party.Count)
	}
	if party.Mons[slot].Fainted() {
		return fmt.Errorf("skill: SwitchActive: slot %d is fainted; the ROM bounces a fainted pick back to the menu", slot)
	}
	want := party.Mons[slot].Species
	if int(m.Peek8(sym.PlayerMonNumber)) == slot {
		// Already out: the ROM would print "ALREADY OUT!" and bounce.
		b := state.DecodeBattle(&mem)
		if b.ActiveSpecies == want {
			return nil
		}
		return fmt.Errorf("skill: SwitchActive: slot %d is marked active but the active species is %#02x, want %#02x", slot, b.ActiveSpecies, want)
	}

	// 1. The FIGHT/ITEM/PKMN/RUN menu: waitBattleMainMenu advances the
	// encounter text that precedes it, exactly as Battle's default branch
	// does, and stops the moment the menu is drawn.
	if err := waitBattleMainMenu(m); err != nil {
		return fmt.Errorf("skill: SwitchActive: %w", err)
	}

	// 2. The POKéMON entry: right column, row 0. The cursor opens at FIGHT
	// (wBattleAndStartSavedMenuItem), but a stale saved item could leave it
	// anywhere in the grid, so every tap is verified against wTopMenuItemX
	// and wCurrentMenuItem before the next one — never a press count.
	atPKMN := func(m *emu.Emu) bool {
		return m.Peek8(sym.TopMenuItemX) == battleMenuRightX && int(m.Peek8(sym.CurrentMenuItem)) == 0
	}
	for i := 0; i < 8; i++ {
		if atPKMN(m) {
			break
		}
		prevX, prevRow := m.Peek8(sym.TopMenuItemX), int(m.Peek8(sym.CurrentMenuItem))
		var btn emu.Button
		switch {
		case prevX == battleMenuLeftX && prevRow != 0:
			btn = emu.Up // ITEM -> FIGHT
		case prevX == battleMenuLeftX:
			btn = emu.Right // FIGHT -> PKMN: RIGHT keeps the row
		default:
			btn = emu.Left // right column: back to the left at the same row
		}
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return m.Peek8(sym.TopMenuItemX) != prevX || int(m.Peek8(sym.CurrentMenuItem)) != prevRow
		}); err != nil {
			return fmt.Errorf("skill: SwitchActive: cursor stuck at x=%#02x row %d, want POKéMON (x=%#02x row 0)",
				prevX, prevRow, battleMenuRightX)
		}
	}
	if !atPKMN(m) {
		return fmt.Errorf("skill: SwitchActive: cursor at x=%#02x row %d, want POKéMON (x=%#02x row 0)",
			m.Peek8(sym.TopMenuItemX), int(m.Peek8(sym.CurrentMenuItem)), battleMenuRightX)
	}

	// 3. A on POKéMON opens the party menu. The VOLUNTARY menu prints the
	// NORMAL_PARTY_MENU footer "Choose a #MON." (core.asm .partyMenuWasSelected
	// sets wPartyMenuTypeOrMessageID to NORMAL_PARTY_MENU); if the FORCED
	// menu's "Bring out" appears instead, the lead fainted and the screen
	// belongs to Battle, not this function — fail rather than drive it.
	for i := 0; i < 24; i++ {
		if battleSwitchMenuUp(m) {
			break
		}
		if partyMenuUp(m) {
			return fmt.Errorf("skill: SwitchActive: the forced switch menu appeared (the lead fainted); that screen belongs to Battle, not a voluntary switch")
		}
		m.Tap(emu.A, 3, 7)
		if _, err := m.StepUntil(25, battleSwitchMenuUp); err == nil {
			break
		}
	}
	if !battleSwitchMenuUp(m) {
		return fmt.Errorf("skill: SwitchActive: the voluntary party menu did not appear after selecting POKéMON")
	}

	// 4. The slot, step-and-verify on wCurrentMenuItem as everywhere else.
	if err := SelectPartySlot(m, slot); err != nil {
		return fmt.Errorf("skill: SwitchActive: %w", err)
	}

	// 5. The SWITCH/STATS/CANCEL box (SWITCH_STATS_CANCEL_MENU_TEMPLATE):
	// A selects SWITCH (index 0). The switch itself is the positive fact —
	// LoadBattleMonFromParty rewrites wBattleMonSpecies before the send-out
	// animation — so the loop ends on the species, never on a press count.
	activeIs := func(m *emu.Emu) bool {
		var s state.Mem
		state.Snapshot(m, &s)
		b := state.DecodeBattle(&s)
		return b != nil && b.ActiveSpecies == want
	}
	for i := 0; i < 24; i++ {
		if activeIs(m) {
			break
		}
		m.Tap(emu.A, 3, 7)
		if _, err := m.StepUntil(25, activeIs); err == nil {
			break
		}
	}

	// Postcondition: the active battle mon's species IS the wanted one.
	state.Snapshot(m, &mem)
	b := state.DecodeBattle(&mem)
	if b == nil || b.ActiveSpecies != want {
		got := uint8(0)
		if b != nil {
			got = b.ActiveSpecies
		}
		return fmt.Errorf("skill: SwitchActive: active species %#02x after selecting slot %d, want %#02x", got, slot, want)
	}
	return nil
}

// SelectPartySlot moves the party menu cursor to index, asserts that
// wCurrentMenuItem reads index, presses A, and waits for the selection to
// take. It is the single slot-selection path for every party menu: Battle's
// forced switch after a faint (the "Bring out" menu), the voluntary switch
// (the "Choose" menu, SwitchActive) and the overworld item-use menu (the
// "Use item on which #MON?" menu, UseFieldItem) go through it rather than
// hand-rolling a second one.
//
// SelectMenuItem cannot do this job: the party menu stores wMaxMenuItem as
// the LAST valid index (count-1, PartyMenuInit), while SelectMenuItem's
// range check treats it as a count, so it rejects the menu's last entry.
// The cursor index is the positive fact, as in SelectMenuItem: each
// direction tap is followed by a re-read of wCurrentMenuItem, and A is
// pressed only once the index is asserted.
//
// The first A can be lost in the menu's joypad-init window — the screen is
// drawn a few frames before HandlePartyMenuInput starts polling (measured
// in PromoteToLead: the first A never lands, the second always does) — so
// A is re-pressed until the menu is gone, never counted.
func SelectPartySlot(m *emu.Emu, index int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	if index < 0 || index >= int(party.Count) {
		return fmt.Errorf("skill: SelectPartySlot: index %d out of range for a party of %d", index, party.Count)
	}

	// Move the cursor toward index and verify after every tap.
	const stuckLimit = 5
	stuck := 0
	for {
		state.Snapshot(m, &mem)
		cur := state.DecodeMenu(&mem).Current
		if cur == index {
			break
		}
		btn := emu.Down
		if cur > index {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return int(m.Peek8(sym.CurrentMenuItem)) != cur
		}); err != nil {
			stuck++
			if stuck >= stuckLimit {
				state.Snapshot(m, &mem)
				return fmt.Errorf("skill: SelectPartySlot: cursor stuck at %d, wanted %d (party of %d), %d consecutive taps without movement",
					state.DecodeMenu(&mem).Current, index, party.Count, stuck)
			}
		} else {
			stuck = 0
		}
	}

	// Press A until the selection took; each re-press is gated on the menu
	// still being up, so a stray A that left it is reported, not chased.
	// The two menus end differently (measured): the forced "Bring out" menu
	// simply goes away, while the voluntary "Choose" menu is covered by the
	// SWITCH/STATS/CANCEL box, which is drawn ON TOP of it — the footer
	// persists in wTileMap under the box, so "menu gone" alone would never
	// fire for the voluntary one.
	selectionTook := func(m *emu.Emu) bool {
		return switchBoxUp(m) || (!partyMenuUp(m) && !battleSwitchMenuUp(m) && !useItemPartyMenuUp(m))
	}
	for i := 0; i < 24; i++ {
		if selectionTook(m) {
			return nil
		}
		m.Tap(emu.A, 3, 7)
		if _, err := m.StepUntil(25, selectionTook); err == nil {
			return nil
		}
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: SelectPartySlot: party menu still up after selecting slot %d: %+v",
		index, state.DecodeMenu(&mem))
}
