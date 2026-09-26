package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// Frame budgets for the bag submenu. The list menu is drawn with a palette
// reload and a 10-frame delay, so a few hundred frames covers the
// transition; the cap exists to fail loudly rather than hang.
const (
	bagMainMenuBudget = 3000 // wait for the ordinary battle action menu after an encounter
	bagMenuBudget     = 500  // wait for the bag list to be drawn
	bagUseBudget      = 3000 // wait for the item's effect (count drop) after A
)

// ErrNotInBag reports that the bag has no entry for the wanted item.
var ErrNotInBag = errors.New("skill: item not in bag")

type itemUseMachine interface {
	menuMachine
	FrameCount() uint64
}

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
	overworld, err := overworldDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: EnterWildBattle: %w", err)
	}
	battle, err := battleStateDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: EnterWildBattle: %w", err)
	}
	live := overworld.DecodeOverworld(m)
	if !live.Controllable {
		return fmt.Errorf("skill: EnterWildBattle: player not controllable on map %#04x", live.NativeMapID)
	}
	now, err := currentWorld(m)
	if err != nil {
		return fmt.Errorf("skill: EnterWildBattle: observe world: %w", err)
	}
	grass, grid, err := grassCells(m.ROM(), now.Map)
	if err != nil {
		return err
	}
	if len(grass) == 0 {
		return fmt.Errorf("skill: EnterWildBattle: no walkable tall grass on map %#04x", now.Map)
	}

	at := cell{int(now.X), int(now.Y)}
	a, b, ok := grindPair(grass, grid, at.x, at.y, spriteBlockers(m))
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
		if waitBattleStartWithDecoder(m, battle, 1000) {
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
	decoder, err := battleStateDecoderFor(m)
	if err != nil {
		return false
	}
	return waitBattleStartWithDecoder(m, decoder, budget)
}

func waitBattleStartWithDecoder(m menuMachine, decoder game.BattleStateDecoder, budget int) bool {
	if decoder == nil {
		return false
	}
	return waitMenuUntil(m, budget, func() bool {
		_, ok := decoder.DecodeBattleState(m)
		return ok
	})
}

// waitBattleMainMenu advances encounter text and animations until the
// profile reports the ordinary battle action menu. It never assumes a
// concrete menu layout or cursor encoding.
func waitBattleMainMenu(m *emu.Emu) error {
	decoder, err := battleMenuDecoderFor(m)
	if err != nil {
		return err
	}
	return waitBattleMainMenuWithDecoder(m, decoder)
}

func waitBattleMainMenuWithDecoder(m itemUseMachine, decoder game.BattleMenuDecoder) error {
	if decoder == nil {
		return fmt.Errorf("skill: UseItem: nil battle-menu decoder")
	}
	start := m.FrameCount()
	for !decoder.DecodeBattleMainMenu(m).Visible {
		if int(m.FrameCount()-start) > bagMainMenuBudget {
			return fmt.Errorf("skill: UseItem: battle main menu did not open within %d frames", bagMainMenuBudget)
		}
		m.Tap(emu.A, 3, 7)
	}
	return nil
}

// UseItem uses one carried item during a battle. The active game profile owns
// battle-menu layout, item-list scrolling, inventory layout, live battle state,
// and prompt identity. Success is proven positively by the item's quantity
// dropping by exactly one.
func UseItem(m *emu.Emu, item uint8) error {
	inventory, err := inventoryDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseItem: %w", err)
	}
	battle, err := battleStateDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseItem: %w", err)
	}
	runtime, err := battleRuntimeDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseItem: %w", err)
	}
	battleMenu, err := battleMenuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseItem: %w", err)
	}
	listMenu, err := listMenuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseItem: %w", err)
	}
	menu, err := menuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseItem: %w", err)
	}
	prompt, err := promptDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: UseItem: %w", err)
	}
	return useItemWithDecoders(m, uint16(item), inventory, battle, runtime, battleMenu, listMenu, menu, prompt)
}

func useItemWithDecoders(
	m itemUseMachine,
	item uint16,
	inventory game.InventoryDecoder,
	battle game.BattleStateDecoder,
	runtime game.BattleRuntimeDecoder,
	battleMenu game.BattleMenuDecoder,
	listMenu game.ListMenuDecoder,
	menu game.MenuDecoder,
	prompt game.PromptDecoder,
) error {
	if m == nil || inventory == nil || battle == nil || runtime == nil || battleMenu == nil || listMenu == nil || menu == nil || prompt == nil {
		return fmt.Errorf("skill: UseItem: incomplete semantic execution capability")
	}
	if _, ok := battle.DecodeBattleState(m); !ok {
		return fmt.Errorf("skill: UseItem: no battle in progress on %s", battleRuntimeContext(runtime.DecodeBattleRuntime(m)))
	}
	if err := waitBattleMainMenuWithDecoder(m, battleMenu); err != nil {
		return err
	}

	idx, before := inventoryEntry(inventory.DecodeInventory(m), item)
	if idx < 0 {
		return fmt.Errorf("skill: UseItem: %w (id %#02x)", ErrNotInBag, item)
	}

	if err := activateBattleMainMenuEntryWithDecoder(m, battleMenu, game.BattleMenuItems, bagMenuBudget, func() bool {
		live := listMenu.DecodeListMenu(m)
		return live.Visible && live.Kind == game.ListMenuItems
	}); err != nil {
		return fmt.Errorf("skill: UseItem: open item list on %s: %w",
			battleRuntimeContext(runtime.DecodeBattleRuntime(m)), err)
	}

	if err := selectScrollingListEntryWithDecoder(m, listMenu, idx); err != nil {
		return fmt.Errorf("skill: UseItem: select bag entry %d: %w", idx, err)
	}

	// A caught Pokémon can ask for a nickname before the ball is removed from
	// inventory. Blindly pressing A there accepts YES and enters the naming
	// keyboard. Only the profile-classified nickname prompt is answered; any
	// other live choice is an ownership failure rather than an implicit choice.
	start := m.FrameCount()
	for int(m.FrameCount()-start) <= bagUseBudget {
		_, after := inventoryEntry(inventory.DecodeInventory(m), item)
		if after == before-1 {
			return nil
		}

		if _, open := menu.DecodeTwoOption(m); open {
			livePrompt := prompt.DecodePrompt(m)
			if livePrompt.Visible && livePrompt.Kind == game.PromptNickname {
				if err := selectTwoOptionWithDecoder(m, menu, 1); err != nil {
					return fmt.Errorf("skill: UseItem: decline caught-Pokemon nickname prompt: %w", err)
				}
				continue
			}
			return fmt.Errorf("skill: UseItem: unexpected choice prompt while resolving item %#02x", item)
		}

		if _, ok := battle.DecodeBattleState(m); !ok {
			return fmt.Errorf(
				"skill: UseItem: battle ended on %s without the count for %#02x dropping from %d",
				battleRuntimeContext(runtime.DecodeBattleRuntime(m)), item, before,
			)
		}
		m.Tap(emu.A, 3, 7)
	}
	_, after := inventoryEntry(inventory.DecodeInventory(m), item)
	return fmt.Errorf("skill: UseItem: bag count for %#02x did not drop from %d (now %d) within %d frames",
		item, before, after, bagUseBudget)
}

// inventoryEntry reports the absolute list position and quantity of an item.
// Item-list ordering is the profile-projected inventory ordering.
func inventoryEntry(state game.InventoryState, item uint16) (int, int) {
	for i, it := range state.Items {
		if it.NativeItemID == item {
			return i, it.Quantity
		}
	}
	return -1, 0
}

// selectBagEntry uses the shared profile-driven scrolling-list driver. The
// caller already resolved the bag entry from inventory order, so only the
// absolute semantic list position is needed here.
func selectBagEntry(m *emu.Emu, idx int) error {
	if err := selectScrollingListEntry(m, idx); err != nil {
		return fmt.Errorf("skill: UseItem: select bag entry %d: %w", idx, err)
	}
	return nil
}
