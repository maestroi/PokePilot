package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const nuggetItem uint8 = 0x31

// safeRareCandyTarget chooses a party member whose next level cannot open a
// level-up move choice or a level-evolution sequence. Bag-pressure recovery
// must never trade a full bag for an unresolved prompt. Among safe choices the
// highest-level healthy member gets the candy, which turns the finite item
// into useful battle strength rather than waste.
func safeRareCandyTarget(romData []byte, party state.PartyState) (int, bool) {
	evos, err := rom.Evolutions(romData)
	if err != nil {
		return -1, false
	}
	bestSlot, bestLevel := -1, -1
	for slot, mon := range party.Mons {
		if mon.Level >= 100 || mon.Fainted() {
			continue
		}
		next := mon.Level + 1
		unsafe := false
		for _, evo := range evos {
			if evo.From == mon.Species && evo.Method == rom.EvoLevel && evo.Level == next {
				unsafe = true
				break
			}
		}
		if unsafe {
			continue
		}
		moves, err := rom.LevelUpMoves(romData, mon.Species)
		if err != nil {
			continue
		}
		for _, move := range moves {
			if move.Level == next {
				unsafe = true
				break
			}
		}
		if unsafe {
			continue
		}
		if int(mon.Level) > bestLevel {
			bestSlot, bestLevel = slot, int(mon.Level)
		}
	}
	return bestSlot, bestSlot >= 0
}

func firstStoredTM(inv state.InventoryState) (uint8, bool) {
	for _, it := range inv.Items {
		if it.ID >= rom.TM01Item && it.ID <= rom.TM50Item && it.Quantity > 0 {
			return it.ID, true
		}
	}
	return 0, false
}

// nearestUsableMart is the selling counterpart to nearestStockMart: selling
// does not care what the clerk stocks, only that an ordinary mart is reachable.
// The standard-mart table includes every standalone Red mart that shares the
// (2,5) player / (1,5) counter geometry.
func nearestUsableMart(romData []byte, mem *state.Mem) (standardMartRecoveryTarget, bool, error) {
	g, err := world.BuildGraph(romData)
	if err != nil {
		return standardMartRecoveryTarget{}, false, err
	}
	from := mem.U8(sym.CurMap)
	bestLen := int(^uint(0) >> 1)
	var best standardMartRecoveryTarget
	found := false
	for _, target := range standardMartRecoveryTargets {
		if target.mapID == viridianMartMap && !state.HasEvent(mem, state.EventOakGotParcel) {
			continue
		}
		route, err := world.FindRoute(g, from, target.mapID)
		if err != nil {
			continue
		}
		if !found || len(route) < bestLen {
			bestLen, best, found = len(route), target, true
		}
	}
	return best, found, nil
}

// runInventoryDetour executes a recovery action that may travel away from the
// caller, then restores the exact map/tile where bag pressure was detected.
// Callers such as Pickup and scripted rewards rely on their local coordinates
// still being valid after EnsureBagSpaceFor returns.
func runInventoryDetour(m *emu.Emu, romData []byte, policy MovePolicy, action func() error) error {
	var before state.Mem
	state.Snapshot(m, &before)
	origin := Destination{Map: before.U8(sym.CurMap), X: before.U8(sym.XCoord), Y: before.U8(sym.YCoord)}

	actionErr := action()
	var after state.Mem
	state.Snapshot(m, &after)
	if !state.Controllable(&after) {
		boundaryErr := fmt.Errorf("skill: inventory detour ended off the controllable overworld on map %#04x at (%d,%d)",
			after.U8(sym.CurMap), after.U8(sym.XCoord), after.U8(sym.YCoord))
		if actionErr != nil {
			return errors.Join(actionErr, boundaryErr)
		}
		return boundaryErr
	}

	if after.U8(sym.CurMap) != origin.Map || after.U8(sym.XCoord) != origin.X || after.U8(sym.YCoord) != origin.Y {
		if _, err := TravelFlee(m, romData, origin, policy, inventoryRecoveryBattles); err != nil {
			return errors.Join(actionErr, fmt.Errorf("skill: inventory detour return to map %#04x at (%d,%d): %w",
				origin.Map, origin.X, origin.Y, err))
		}
	}
	return actionErr
}

func sellNuggetsForBagSpace(m *emu.Emu, romData []byte, policy MovePolicy) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	_, qty := bagEntry(&mem, nuggetItem)
	if qty == 0 || state.HasEvent(&mem, eventInSafariZone) {
		return false, nil
	}
	mart, ok, err := nearestUsableMart(romData, &mem)
	if err != nil || !ok {
		return false, err
	}
	err = runInventoryDetour(m, romData, policy, func() error {
		dest := Destination{Map: mart.mapID, X: 2, Y: 5}
		if _, err := TravelFlee(m, romData, dest, policy, inventoryRecoveryBattles); err != nil {
			return fmt.Errorf("skill: bag pressure: reach %s to sell NUGGET: %w", mart.name, err)
		}
		if err := Face(m, 1, 5); err != nil {
			return fmt.Errorf("skill: bag pressure: face %s counter: %w", mart.name, err)
		}
		if err := Sell(m, nuggetItem, qty); err != nil {
			return fmt.Errorf("skill: bag pressure: sell NUGGET x%d at %s: %w", qty, mart.name, err)
		}
		return nil
	})
	return err == nil, err
}

func useRareCandyForBagSpace(m *emu.Emu, romData []byte) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	_, qty := bagEntry(&mem, rareCandyItem)
	// Using one candy from a larger stack does not free a distinct bag slot.
	// Do not burn a whole reserve merely to manufacture capacity.
	if qty != 1 {
		return false, nil
	}
	slot, ok := safeRareCandyTarget(romData, state.DecodeParty(&mem))
	if !ok {
		return false, nil
	}
	if err := UseFieldItem(m, rareCandyItem, slot); err != nil {
		return false, fmt.Errorf("skill: bag pressure: use RARE CANDY on party slot %d: %w", slot, err)
	}
	return true, nil
}

func storeTMForBagSpace(m *emu.Emu, romData []byte, policy MovePolicy) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.HasEvent(&mem, eventInSafariZone) {
		return false, nil
	}
	item, ok := firstStoredTM(state.DecodeInventory(&mem))
	if !ok {
		return false, nil
	}
	err := runInventoryDetour(m, romData, policy, func() error {
		return DepositBagStack(m, romData, policy, item)
	})
	if err != nil {
		return false, fmt.Errorf("skill: bag pressure: store TM %#02x in Player PC: %w", item, err)
	}
	return true, nil
}

// ensureBagFreeSlotsManaged resolves bag pressure with productive actions before
// discarding anything. NUGGET is converted to money, a single safe RARE CANDY
// is converted to a level, and a finite TM can be preserved in Player PC
// storage. Only when none of those can create the required capacity does the
// legacy explicit toss whitelist run.
func ensureBagFreeSlotsManaged(m *emu.Emu, minFree int) error {
	if minFree < 0 || minFree > gen1BagCapacity {
		return fmt.Errorf("skill: ensureBagFreeSlotsManaged: requested %d free slots, want 0..%d", minFree, gen1BagCapacity)
	}
	if minFree == 0 {
		return nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if bagFreeSlots(&mem) >= minFree {
		return nil
	}
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: bag pressure: player not controllable on map %#04x at (%d,%d)",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}

	romData := m.ROM()
	policy := StatAwareMove(romData)

	// A Nugget is pure money once acquired. Sell the whole stack whenever bag
	// pressure has already forced inventory maintenance.
	if _, qty := bagEntry(&mem, nuggetItem); qty > 0 {
		if _, err := sellNuggetsForBagSpace(m, romData, policy); err != nil {
			// If no transaction occurred but another productive action can still
			// satisfy capacity, do not turn an unreachable mart into a blocker.
			state.Snapshot(m, &mem)
			if bagFreeSlots(&mem) < minFree {
				// Continue to local/store/toss recovery below.
			}
		}
	}

	// Rare Candy is useful rather than disposable. Even if selling the Nugget
	// already made the mandatory slot, consume one safe singleton Candy now so
	// future rewards do not immediately recreate the same 20/20 pressure.
	state.Snapshot(m, &mem)
	if _, qty := bagEntry(&mem, rareCandyItem); qty == 1 {
		if _, err := useRareCandyForBagSpace(m, romData); err != nil {
			return err
		}
	}

	state.Snapshot(m, &mem)
	if bagFreeSlots(&mem) >= minFree {
		return nil
	}

	// Preserve a finite TM in Player PC before destroying replenishable stock.
	if _, err := storeTMForBagSpace(m, romData, policy); err != nil {
		// Storage is a preservation optimization. If it cannot be completed
		// cleanly, fall through to the explicit toss whitelist rather than
		// blocking progression solely on optional PC capacity.
		state.Snapshot(m, &mem)
		if !state.Controllable(&mem) {
			return err
		}
	}
	state.Snapshot(m, &mem)
	if bagFreeSlots(&mem) >= minFree {
		return nil
	}

	return EnsureBagFreeSlots(m, minFree)
}
