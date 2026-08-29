package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// Frame budgets for the bag submenu. The list menu is drawn with a palette
// reload and a 10-frame delay, so a few hundred frames covers the
// transition; the cap exists to fail loudly rather than hang.
const (
	bagMainMenuBudget = 3000 // wait for the FIGHT/ITEM/PKMN/RUN menu after an encounter
	bagMenuBudget     = 500  // wait for the bag list to be drawn
	bagUseBudget      = 3000 // wait for the item's effect (count drop) after A
)

// ErrNotInBag reports that the bag has no entry for the wanted item.
var ErrNotInBag = errors.New("skill: item not in bag")

// EnterWildBattle steps the player into the tall grass on the current map
// and returns once a wild battle is in progress. It walks to the nearest
// walkable grass cell, then steps out of it and back in until the game
// rolls an encounter: every step onto grass re-rolls, so attempts bounds
// the number of entries, not frames. The battle may start anywhere on the
// walk in, which is fine — the caller only needs a fight in progress.
//
// It returns an error if the map has no reachable grass or no encounter
// rolled within attempts entries; it never fights or ends the battle.
func EnterWildBattle(m *emu.Emu, attempts int) error {
	if attempts <= 0 {
		return fmt.Errorf("skill: EnterWildBattle: attempts must be > 0, got %d", attempts)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: EnterWildBattle: player not controllable on map %#04x", m.Peek8(sym.CurMap))
	}
	now := currentWorld(m)
	grass, grid, err := grassCells(m.ROM(), now.Map)
	if err != nil {
		return err
	}
	if len(grass) == 0 {
		return fmt.Errorf("skill: EnterWildBattle: no walkable tall grass on map %#04x", now.Map)
	}

	at := cell{int(now.X), int(now.Y)}
	a, b, ok := grindPair(grass, grid, at.x, at.y)
	if !ok {
		return fmt.Errorf("skill: EnterWildBattle: map %#04x has no two walkable grass cells close enough to ping-pong between", now.Map)
	}

	// Ping-pong the two grass cells with GoTo. Each leg ends by stepping
	// onto a fresh grass cell, which re-rolls the encounter; GoTo aborts
	// with ErrBattle the moment one fires, leaving the battle in progress.
	next := b
	legs := 0
	for {
		d := Destination{Map: now.Map, X: uint8(next.x), Y: uint8(next.y)}
		if err := GoTo(m, m.ROM(), d); err != nil && !errors.Is(err, ErrBattle) {
			return fmt.Errorf("skill: EnterWildBattle: walk to grass cell (%d,%d): %w", next.x, next.y, err)
		}
		if waitBattleStart(m, 1000) {
			return nil
		}
		legs++
		if legs > attempts {
			return fmt.Errorf("skill: EnterWildBattle: no wild encounter after %d grass legs on map %#04x", attempts, now.Map)
		}
		next = flip(a, b, next)
	}
}

// waitBattleStart steps until a battle is in progress and reports whether
// one started within budget frames.
func waitBattleStart(m *emu.Emu, budget int) bool {
	if _, err := m.StepUntil(budget, battleInFlight); err != nil {
		return false
	}
	return true
}

// waitBattleMainMenu advances the encounter text and animations until the
// FIGHT/ITEM/PKMN/RUN menu is up. The "A wild X appeared!" box does not
// auto-advance, so — exactly as Battle's default branch does — each pass
// taps A to move it along. It stops the moment the main menu is drawn, so
// it never presses A on top of the menu itself (which would select FIGHT).
func waitBattleMainMenu(m *emu.Emu) error {
	start := m.FrameCount()
	for !mainMenuUp(m) {
		if int(m.FrameCount()-start) > bagMainMenuBudget {
			return fmt.Errorf("skill: UseItem: battle main menu did not open within %d frames", bagMainMenuBudget)
		}
		m.Tap(emu.A, 3, 7)
	}
	return nil
}

// UseItem uses one of item from the bag during a battle. From an open
// battle main menu it opens ITEM, moves the cursor to the bag's entry for
// item, and uses it. Its postcondition is that the bag's count for item
// DROPPED by one, read back via state.DecodeInventory; if the game declines
// the item the count never drops and UseItem reports that.
//
// The bag list scrolls: the visible window is four entries plus CANCEL, and
// wMaxMenuItem holds only the window size (1 or 2), so the entry under the
// cursor is wListScrollOffset + wCurrentMenuItem, not wCurrentMenuItem
// alone. Selection is step-and-verify on that position — press, assert it
// moved, repeat until it equals the wanted index, then A — never a press
// count.
func UseItem(m *emu.Emu, item uint8) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) == nil {
		x, y := playerXY(m)
		return fmt.Errorf("skill: UseItem: no battle in progress on map %02x at (%d,%d)", m.Peek8(sym.CurMap), x, y)
	}
	if err := waitBattleMainMenu(m); err != nil {
		return err
	}

	state.Snapshot(m, &mem)
	idx, before := bagEntry(&mem, item)
	if idx < 0 {
		return fmt.Errorf("skill: UseItem: %w (id %#02x)", ErrNotInBag, item)
	}

	// ITEM is the second entry of the main menu (below FIGHT). The menu is
	// a 2x2 grid with wMaxMenuItem == 1 per column, so SelectMenuItem would
	// reject index 1 as out of range; step-and-verify it by hand.
	m.Tap(emu.Down, 3, 7)
	if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool { return int(m.Peek8(sym.CurrentMenuItem)) == 1 }); err != nil {
		return fmt.Errorf("skill: UseItem: cursor did not reach the ITEM entry: %w", err)
	}
	m.Tap(emu.A, 3, 7)

	// The bag list is identified from wTileMap like the other battle menus:
	// CANCEL is drawn by the list menu and appears on no other battle screen
	// (measured against a live battle; wFontLoaded/wMaxMenuItem are useless
	// here — the list menu sets wMaxMenuItem to the window size, not the
	// entry count).
	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool { return battleScreenHas(m, bagMenuMarker) }); err != nil {
		return fmt.Errorf("skill: UseItem: bag list did not open within %d frames", bagMenuBudget)
	}

	if err := selectBagEntry(m, idx); err != nil {
		return err
	}

	// Postcondition: the count dropped by one, read back from RAM — a nil
	// return is not evidence the item was used. The drop is NOT immediate:
	// the game writes it only as the "used X!" text and (for a ball) the
	// catch sequence resolve, and that text does not auto-advance. So each
	// pass taps A to drive it along, exactly as waitBattleMainMenu does, and
	// stops the instant the count is observed to have dropped — before any
	// A can land on the FIGHT menu the sequence returns to.
	start := m.FrameCount()
	for {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if _, after := bagEntry(&mem, item); after == before-1 {
			return nil
		}
		if state.DecodeBattle(&mem) == nil {
			// A successful catch ends the battle; the drop is observed at the
			// result text, before this point. Ending without it means the
			// item was not consumed — stop rather than tap A in the overworld.
			x, y := playerXY(m)
			return fmt.Errorf("skill: UseItem: battle ended on map %02x at (%d,%d) without the count for %#02x dropping from %d", m.Peek8(sym.CurMap), x, y, item, before)
		}
		if int(m.FrameCount()-start) > bagUseBudget {
			_, after := bagEntry(&mem, item)
			return fmt.Errorf("skill: UseItem: bag count for %#02x did not drop from %d (now %d) within %d frames", item, before, after, bagUseBudget)
		}
		m.Tap(emu.A, 3, 7)
	}
}

// bagMenuMarker identifies the battle bag list in wTileMap.
const bagMenuMarker = "CANCEL"

// bagEntry reports the index of the bag's entry for item and its quantity.
// The battle bag lists the bag's entries in order, so the list position is
// the slice index. It returns -1 when the bag holds no such item.
func bagEntry(mem *state.Mem, item uint8) (int, int) {
	for i, it := range state.DecodeInventory(mem).Items {
		if it.ID == item {
			return i, int(it.Quantity)
		}
	}
	return -1, 0
}

// bagPosition is the entry under the cursor: the list menu scrolls by
// raising wListScrollOffset while the cursor stays put at the window's
// bottom, so the visible index is the sum.
func bagPosition(m *emu.Emu) int {
	return int(m.Peek8(sym.ListScrollOffset)) + int(m.Peek8(sym.CurrentMenuItem))
}

// selectBagEntry moves the cursor of the open bag list to entry idx and
// presses A, step-and-verify: each tap is followed by a re-read of the
// position, and A is pressed only once it is asserted to be idx. It never
// relies on wrap-around or a press count, so it works across the scroll
// boundary where wCurrentMenuItem stops moving and wListScrollOffset takes
// over.
func selectBagEntry(m *emu.Emu, idx int) error {
	const stuckLimit = 5
	stuck := 0
	for pos := bagPosition(m); pos != idx; {
		btn := emu.Down
		if pos > idx {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool { return bagPosition(m) != pos }); err != nil {
			stuck++
			if stuck >= stuckLimit {
				return fmt.Errorf("skill: UseItem: bag cursor stuck at entry %d, wanted %d, %d consecutive taps without movement: %w", pos, idx, stuck, ErrMenuStuck)
			}
		} else {
			stuck = 0
		}
		pos = bagPosition(m)
	}
	m.Tap(emu.A, 3, 7)
	return nil
}
