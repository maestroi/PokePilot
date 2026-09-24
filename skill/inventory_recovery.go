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

const (
	progressionPokeBallReserve = 10
	inventoryRecoveryBattles   = 100
)

var ErrNoReachableStock = errors.New("skill: no reachable mart stocks the required item")

// standardMartRecoveryTarget describes the ordinary Gen I mart layout used by
// the early-game shops. The clerk stands behind the counter at (0,5); the
// player stands at (2,5) and faces the counter tile at (1,5). Pressing A from
// there reaches the clerk through the counter, which is exactly the boundary
// Buy expects.
type standardMartRecoveryTarget struct {
	name  string
	mapID uint8
}

var standardMartRecoveryTargets = []standardMartRecoveryTarget{
	{name: "viridian mart", mapID: 0x2A},
	{name: "pewter mart", mapID: 0x38},
	{name: "cerulean mart", mapID: 0x43},
	{name: "vermilion mart", mapID: 0x5B},
	{name: "lavender mart", mapID: 0x96},
	{name: "fuchsia mart", mapID: 0x98},
	{name: "cinnabar mart", mapID: 0xAC},
	{name: "saffron mart", mapID: 0xB4},
	// The League lobby shop shares the ordinary counter geometry, and it is
	// the only shop reachable from the post-loss Indigo checkpoint without
	// backtracking through Victory Road.
	{name: "indigo plateau mart", mapID: 0xAE},
}

func itemCount(mem *state.Mem, item uint8) int {
	_, qty := bagEntry(mem, item)
	return qty
}

func martStocksItem(romData []byte, mapID, item uint8) bool {
	items, err := rom.MartItems(romData, mapID)
	if err != nil {
		return false
	}
	for _, stocked := range items {
		if stocked == item {
			return true
		}
	}
	return false
}

// reachableStockMarts returns the recovery marts GoTo can route to right now,
// with their map-hop distance. Reachability applies the same capability gates
// travel enforces (e.g. the League rooms cannot be left mid-challenge), so a
// restock is never offered from a place the route will then refuse.
func reachableStockMarts(romData []byte, mem *state.Mem) (map[standardMartRecoveryTarget]int, error) {
	g, err := cachedRouteGraph(romData)
	if err != nil {
		return nil, err
	}
	if g, err = withAsleepRoute16Snorlax(g, romData, mem); err != nil {
		return nil, err
	}
	prereqs := redRoutePrerequisites(g, romData, mem)
	from, x, y := mem.U8(sym.CurMap), int(mem.U8(sym.XCoord)), int(mem.U8(sym.YCoord))
	out := map[standardMartRecoveryTarget]int{}
	for _, target := range standardMartRecoveryTargets {
		// Before Oak receives the parcel the Viridian clerk is a story NPC,
		// not a usable shop. Do not route a recovery there just to learn that
		// again from the shop controller.
		if target.mapID == viridianMartMap && !state.HasEvent(mem, state.EventOakGotParcel) {
			continue
		}
		if target.mapID == from {
			out[target] = 0
			continue
		}
		route, err := world.FindRoutePlanAtDestinationWithCapabilities(g, from, target.mapID, x, y, -1, -1, nil, prereqs)
		if err != nil && !errors.Is(err, world.ErrRouteReplanRequired) {
			continue
		}
		out[target] = len(route)
	}
	return out, nil
}

func nearestStockMart(romData []byte, mem *state.Mem, item uint8) (standardMartRecoveryTarget, bool, error) {
	marts, err := reachableStockMarts(romData, mem)
	if err != nil {
		return standardMartRecoveryTarget{}, false, err
	}
	bestLen := int(^uint(0) >> 1)
	var best standardMartRecoveryTarget
	found := false
	for _, target := range standardMartRecoveryTargets {
		hops, ok := marts[target]
		if !ok || !martStocksItem(romData, target.mapID, item) {
			continue
		}
		if !found || hops < bestLen {
			bestLen, best, found = hops, target, true
		}
	}
	return best, found, nil
}

// NearestMartStock lists what the nearest recovery mart reachable from the
// live state stocks. It lets planners offer "travel and buy" work (RestockItem)
// without standing in a shop, and keeps that offer on the closest counter —
// from the Indigo Plateau checkpoint that is the League lobby shop.
func NearestMartStock(m *emu.Emu, romData []byte) []uint8 {
	var mem state.Mem
	state.Snapshot(m, &mem)
	marts, err := reachableStockMarts(romData, &mem)
	if err != nil {
		return nil
	}
	best, bestHops := standardMartRecoveryTarget{}, -1
	for _, target := range standardMartRecoveryTargets {
		if hops, ok := marts[target]; ok && (bestHops < 0 || hops < bestHops) {
			best, bestHops = target, hops
		}
	}
	if bestHops < 0 {
		return nil
	}
	items, err := rom.MartItems(romData, best.mapID)
	if err != nil {
		return nil
	}
	return items
}

// RestockItem buys qty more of item at the nearest reachable mart that stocks
// it. EnsureItemStock owns travel, bag space, purchase and the bag-count
// postcondition; at least one unit must be added.
func RestockItem(m *emu.Emu, romData []byte, policy MovePolicy, item uint8, qty int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	have := itemCount(&mem, item)
	target := have + qty
	if target > 99 {
		target = 99
	}
	if target <= have {
		return fmt.Errorf("skill: RestockItem: item %#02x already at %d", item, have)
	}
	_, err := EnsureItemStock(m, romData, policy, item, target, have+1)
	return err
}

// EnsureItemStock restores a deterministic reserve before a skill consumes an
// item. target is the preferred quantity and minimum is the hard quantity
// required to continue. This distinction matters for progression: a larger
// catch reserve is safer, but an otherwise valid run should not be blocked
// merely because it can only afford a smaller quantity.
//
// The function owns the whole recovery transaction: choose a reachable mart
// that actually stocks the item according to the ROM, make bag space, travel
// to the counter, buy the largest affordable quantity up to target, and verify
// the bag afterwards. If the existing bag already satisfies minimum, a failed
// top-up is soft and the caller may continue with what it has.
func EnsureItemStock(m *emu.Emu, romData []byte, policy MovePolicy, item uint8, target, minimum int) (int, error) {
	if target < 1 || target > 99 {
		return 0, fmt.Errorf("skill: EnsureItemStock: target %d out of range 1..99", target)
	}
	if minimum < 1 || minimum > target {
		return 0, fmt.Errorf("skill: EnsureItemStock: minimum %d out of range 1..target(%d)", minimum, target)
	}
	if policy == nil {
		return 0, fmt.Errorf("skill: EnsureItemStock: nil move policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	have := itemCount(&mem, item)
	if have >= target {
		return have, nil
	}

	softFail := func(err error) (int, error) {
		state.Snapshot(m, &mem)
		have = itemCount(&mem, item)
		if have >= minimum {
			return have, nil
		}
		return have, err
	}

	mart, ok, err := nearestStockMart(romData, &mem, item)
	if err != nil {
		return softFail(fmt.Errorf("skill: EnsureItemStock: choose mart for item %#02x: %w", item, err))
	}
	if !ok {
		return softFail(fmt.Errorf("%w: item %#02x from map %#04x", ErrNoReachableStock, item, mem.U8(sym.CurMap)))
	}

	if err := EnsureBagSpaceFor(m, item); err != nil {
		return softFail(fmt.Errorf("skill: EnsureItemStock: make room for item %#02x: %w", item, err))
	}

	// Ordinary marts share the same counter geometry. Travel to the open
	// floor in front of the counter, then face the counter tile rather than
	// the clerk: Face requires an adjacent tile, while the game deliberately
	// lets A talk through the counter to the clerk one tile farther away.
	dest := Destination{Map: mart.mapID, X: 2, Y: 5}
	if _, err := TravelFlee(m, romData, dest, policy, inventoryRecoveryBattles); err != nil {
		return softFail(fmt.Errorf("skill: EnsureItemStock: reach %s: %w", mart.name, err))
	}
	if err := Face(m, 1, 5); err != nil {
		return softFail(fmt.Errorf("skill: EnsureItemStock: face %s counter: %w", mart.name, err))
	}

	state.Snapshot(m, &mem)
	have = itemCount(&mem, item)
	need := target - have
	if need <= 0 {
		return have, nil
	}
	if need > 99 {
		need = 99
	}

	var lastErr error
	for qty := need; qty >= 1; qty-- {
		err := Buy(m, item, qty)
		if err == nil {
			break
		}
		lastErr = err
		if !errors.Is(err, ErrCantAfford) {
			return softFail(fmt.Errorf("skill: EnsureItemStock: buy item %#02x x%d at %s: %w", item, qty, mart.name, err))
		}
		// ErrCantAfford guarantees Buy backed out to the overworld. Try a
		// smaller quantity so low money degrades the reserve instead of
		// turning a recoverable catch into a planner failure.
	}

	state.Snapshot(m, &mem)
	have = itemCount(&mem, item)
	if have >= minimum {
		return have, nil
	}
	if lastErr != nil {
		return have, fmt.Errorf("skill: EnsureItemStock: item %#02x remains at %d (need at least %d): %w", item, have, minimum, lastErr)
	}
	return have, fmt.Errorf("skill: EnsureItemStock: item %#02x remains at %d, need at least %d", item, have, minimum)
}

// EnsureProgressionPokeBalls handles the hard zero-ball prerequisite for a
// deterministic progression catch. If at least one ball is already present we
// avoid an unnecessary shopping detour. When the bag is dry, recovery aims for
// ten balls (Catch itself already caps utility catches at ten) and gracefully
// falls back to the largest affordable quantity, with one ball as the hard
// minimum needed to continue.
func EnsureProgressionPokeBalls(m *emu.Emu, romData []byte, policy MovePolicy) (int, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if have := itemCount(&mem, ItemPokeBall); have > 0 {
		return have, nil
	}
	return EnsureItemStock(m, romData, policy, ItemPokeBall, progressionPokeBallReserve, 1)
}
