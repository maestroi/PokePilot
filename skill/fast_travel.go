package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	escapeRopeItem       uint8 = 0x1d
	digMoveID            uint8 = 0x5b
	digFieldMoveMenuID   uint8 = 7
	plateauTileset       uint8 = 23
	agathasRoomMap       uint8 = 0xf7
	fastTravelWarpBudget       = 6000

	// Route-cost units deliberately compare coarse journey exposure rather
	// than emulator frames. One ordinary map transition costs 100. Menu-driven
	// shortcuts carry a fixed action/risk cost; Escape Rope is more expensive
	// because it consumes inventory. A shortcut must be strictly cheaper than
	// the ordinary route, so ties always preserve walking.
	fastTravelMapTransitionCost = 100
	fastTravelFlyActionCost     = 150
	fastTravelDigActionCost     = 90
	fastTravelEscapeActionCost  = 160
)

type fastTravelKind uint8

const (
	fastTravelNone fastTravelKind = iota
	fastTravelFly
	fastTravelDig
	fastTravelEscapeRope
)

// fastTravelOption is the game-adapter contribution to route-cost planning.
// Landing and ActionCost are generic planner inputs; Kind is only the Red
// executor identity. Future adapters can expose equivalent options without
// teaching the world graph about menus, HMs, or consumable item IDs.
type fastTravelOption struct {
	Kind       fastTravelKind
	Landing    Destination
	ActionCost int
}

type fastTravelChoice struct {
	Kind    fastTravelKind
	Map     uint8
	Landing Destination
	Cost    int
}

var flyLanding = map[uint8]Destination{
	0x00: {Map: 0x00, X: 5, Y: 6},
	0x01: {Map: 0x01, X: 23, Y: 26},
	0x02: {Map: 0x02, X: 13, Y: 26},
	0x03: {Map: 0x03, X: 19, Y: 18},
	0x04: {Map: 0x04, X: 3, Y: 6},
	0x05: {Map: 0x05, X: 11, Y: 4},
	0x06: {Map: 0x06, X: 41, Y: 10},
	0x07: {Map: 0x07, X: 19, Y: 28},
	0x08: {Map: 0x08, X: 11, Y: 12},
	0x09: {Map: 0x09, X: 9, Y: 6},
	0x0a: {Map: 0x0a, X: 9, Y: 30},
}

func townVisited(mem *state.Mem, mapID uint8) bool {
	if mem == nil || mapID >= 11 {
		return false
	}
	addr := sym.TownVisitedFlag + uint16(mapID)/8
	return mem.U8(addr)&(1<<(mapID%8)) != 0
}

func outsideForFly(mem *state.Mem) bool {
	if mem == nil {
		return false
	}
	tileset := mem.U8(sym.CurMapTileset)
	return tileset == overworldTileset || tileset == plateauTileset
}

// escapeTravelAllowed mirrors ItemUseEscapeRope: the move/item works only in
// the five declared tilesets, never in battle, and is explicitly disabled in
// Agatha's room even though that room uses the otherwise-legal CEMETERY set.
func escapeTravelAllowed(mem *state.Mem) bool {
	if mem == nil || mem.U8(sym.CurMap) == agathasRoomMap || state.DecodeBattle(mem) != nil {
		return false
	}
	switch mem.U8(sym.CurMapTileset) {
	case 3, 15, 16, 17, 22: // FOREST, CEMETERY, INTERIOR, CAVERN, FACILITY
		return true
	default:
		return false
	}
}

// legalFastTravelOptions projects Red's current RAM into optional route edges.
// Missing badges, learned moves, visited towns, Dig users, or Escape Ropes
// simply omit an option; Travel never enters a recovery loop to manufacture an
// optimization prerequisite.
func legalFastTravelOptions(mem *state.Mem) []fastTravelOption {
	if mem == nil || !state.Controllable(mem) {
		return nil
	}

	var out []fastTravelOption
	if outsideForFly(mem) && townVisited(mem, 0) && FieldCapabilityFor(mem, FieldFly).Usable {
		cur := mem.U8(sym.CurMap)
		for mapID := uint8(0); mapID < 11; mapID++ {
			landing, ok := flyLanding[mapID]
			if !ok || mapID == cur || !townVisited(mem, mapID) {
				continue
			}
			out = append(out, fastTravelOption{
				Kind:       fastTravelFly,
				Landing:    landing,
				ActionCost: fastTravelFlyActionCost,
			})
		}
	}

	if escapeTravelAllowed(mem) {
		if landing, ok := flyLanding[mem.U8(sym.LastBlackoutMap)]; ok {
			if partyMoveSlot(mem, digMoveID) >= 0 {
				out = append(out, fastTravelOption{
					Kind:       fastTravelDig,
					Landing:    landing,
					ActionCost: fastTravelDigActionCost,
				})
			}
			if _, qty := bagEntry(mem, escapeRopeItem); qty > 0 {
				out = append(out, fastTravelOption{
					Kind:       fastTravelEscapeRope,
					Landing:    landing,
					ActionCost: fastTravelEscapeActionCost,
				})
			}
		}
	}
	return out
}

// routeCostFrom estimates journey cost with the same component-aware semantic
// route planner GoTo uses. Map transitions dominate encounter/retry exposure;
// when source and destination are on the same map, Manhattan distance breaks
// the otherwise-zero-cost tie and prevents a Fly landing across town from
// looking free.
func routeCostFrom(p *RoutePlanner, from, dest Destination) (int, bool) {
	if p == nil || p.graph == nil {
		return 0, false
	}
	plan, err := world.FindRoutePlanAtDestinationWithCapabilities(
		p.graph,
		from.Map,
		dest.Map,
		int(from.X),
		int(from.Y),
		int(dest.X),
		int(dest.Y),
		nil,
		p.prereqs,
	)
	if err != nil {
		return 0, false
	}
	cost := len(plan) * fastTravelMapTransitionCost
	if from.Map == dest.Map {
		dx := int(from.X) - int(dest.X)
		if dx < 0 {
			dx = -dx
		}
		dy := int(from.Y) - int(dest.Y)
		if dy < 0 {
			dy = -dy
		}
		cost += dx + dy
	}
	return cost, true
}

// chooseFastTravelByCost is the generic shortcut selector. onwardCost answers
// the ordinary route cost after a candidate landing. Walking wins ties.
func chooseFastTravelByCost(
	walkCost int,
	walkOK bool,
	options []fastTravelOption,
	onwardCost func(Destination) (int, bool),
) fastTravelChoice {
	bestCost := int(^uint(0) >> 1)
	if walkOK {
		bestCost = walkCost
	}
	var best fastTravelChoice
	for _, option := range options {
		onward, ok := onwardCost(option.Landing)
		if !ok {
			continue
		}
		total := option.ActionCost + onward
		if total >= bestCost {
			continue
		}
		bestCost = total
		best = fastTravelChoice{
			Kind:    option.Kind,
			Map:     option.Landing.Map,
			Landing: option.Landing,
			Cost:    total,
		}
	}
	return best
}

// chooseFastTravel turns legal Red shortcuts into route-cost alternatives.
// Fly may land in any visited town that reduces the remaining route; Dig and
// Escape Rope may return to the last healing town and continue onward. They are
// no longer restricted to direct-map destinations and are never forced merely
// because they are available.
func chooseFastTravel(m *emu.Emu, romData []byte, mem *state.Mem, dest Destination) fastTravelChoice {
	if m == nil || mem == nil || mem.U8(sym.CurMap) == dest.Map || !state.Controllable(mem) {
		return fastTravelChoice{}
	}
	options := legalFastTravelOptions(mem)
	if len(options) == 0 {
		return fastTravelChoice{}
	}
	planner, err := NewRoutePlanner(m, romData)
	if err != nil {
		// Fast travel is an optimization. Failure to price it must never turn
		// an otherwise-normal Travel call into a controller failure.
		return fastTravelChoice{}
	}
	from := Destination{Map: planner.cur, X: planner.x, Y: planner.y}
	walkCost, walkOK := routeCostFrom(planner, from, dest)
	return chooseFastTravelByCost(walkCost, walkOK, options, func(landing Destination) (int, bool) {
		return routeCostFrom(planner, landing, dest)
	})
}

func openPartyFieldMove(m *emu.Emu, partySlot int, menuID uint8) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return fmt.Errorf("player is not controllable")
	}
	wantMax, pokemonIndex := startMenuShape(&mem)
	if err := openStartMenuEntry(m, pokemonIndex-1, wantMax); err != nil {
		return fmt.Errorf("open POKEMON: %w", err)
	}
	if _, err := m.StepUntil(1000, normalPartyMenuUp); err != nil {
		return fmt.Errorf("party menu did not appear")
	}
	if err := selectFieldMoveUser(m, partySlot); err != nil {
		return fmt.Errorf("select party slot %d: %w", partySlot, err)
	}
	idx := fieldMoveMenuIndex(m, menuID)
	if idx < 0 {
		return fmt.Errorf("field menu id %d absent for party slot %d", menuID, partySlot)
	}
	if err := SelectMenuItem(m, idx); err != nil {
		return fmt.Errorf("select field menu id %d: %w", menuID, err)
	}
	return nil
}

func waitFastTravelArrival(m *emu.Emu, landing Destination) error {
	if _, err := m.StepUntil(fastTravelWarpBudget, func(e *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(e, &mem)
		return mem.U8(sym.CurMap) == landing.Map && state.Controllable(&mem)
	}); err != nil {
		x, y := playerXY(m)
		return fmt.Errorf("fast travel did not reach map %#02x within %d frames; map=%#02x at (%d,%d)",
			landing.Map, fastTravelWarpBudget, m.Peek8(sym.CurMap), x, y)
	}
	if err := waitForPositionStable(m, positionStableBudget, positionStableFrames); err != nil {
		return err
	}
	x, y := playerXY(m)
	if x != landing.X || y != landing.Y {
		return fmt.Errorf("fast travel to map %#02x landed at (%d,%d), want (%d,%d)",
			landing.Map, x, y, landing.X, landing.Y)
	}
	return nil
}

func useFlyTo(m *emu.Emu, destMap uint8) error {
	landing, ok := flyLanding[destMap]
	if !ok {
		return fmt.Errorf("map %#02x has no Fly landing", destMap)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	cap := FieldCapabilityFor(&mem, FieldFly)
	if !cap.Usable || cap.PartySlot < 0 {
		return fmt.Errorf("Fly is not currently usable")
	}
	if !outsideForFly(&mem) || !townVisited(&mem, 0) || !townVisited(&mem, destMap) {
		return fmt.Errorf("Fly destination %#02x is not currently legal", destMap)
	}
	if err := openPartyFieldMove(m, cap.PartySlot, fieldFlyMenuID); err != nil {
		return fmt.Errorf("Fly: %w", err)
	}

	// ChooseFlyDestination builds 11 city entries plus a trailing $ff. Pallet
	// is the initial cursor; one UP press advances to the next *visited* city,
	// automatically skipping unvisited entries.
	if _, err := m.StepUntil(1000, func(e *emu.Emu) bool {
		return e.Peek8(sym.FlyLocationsList+11) == 0xff
	}); err != nil {
		return fmt.Errorf("Fly destination list did not appear")
	}
	presses := 0
	for id := uint8(1); id <= destMap; id++ {
		if townVisited(&mem, id) {
			presses++
		}
	}
	for i := 0; i < presses; i++ {
		m.Tap(emu.Up, 3, 15)
	}
	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(1000, func(e *emu.Emu) bool {
		return e.Peek8(sym.DestinationMap) == destMap
	}); err != nil {
		return fmt.Errorf("Fly selected map %#02x but wDestinationMap did not agree", destMap)
	}
	return waitFastTravelArrival(m, landing)
}

func useDigFastTravel(m *emu.Emu, destMap uint8) error {
	landing, ok := flyLanding[destMap]
	if !ok {
		return fmt.Errorf("Dig destination map %#02x has no special-warp landing", destMap)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	slot := partyMoveSlot(&mem, digMoveID)
	if slot < 0 || !escapeTravelAllowed(&mem) || mem.U8(sym.LastBlackoutMap) != destMap {
		return fmt.Errorf("Dig fast travel is not legal to map %#02x", destMap)
	}
	if err := openPartyFieldMove(m, slot, digFieldMoveMenuID); err != nil {
		return fmt.Errorf("Dig: %w", err)
	}
	return waitFastTravelArrival(m, landing)
}

func useEscapeRopeFastTravel(m *emu.Emu, destMap uint8) error {
	landing, ok := flyLanding[destMap]
	if !ok {
		return fmt.Errorf("Escape Rope destination map %#02x has no special-warp landing", destMap)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !escapeTravelAllowed(&mem) || mem.U8(sym.LastBlackoutMap) != destMap {
		return fmt.Errorf("Escape Rope fast travel is not legal to map %#02x", destMap)
	}
	if _, qty := bagEntry(&mem, escapeRopeItem); qty <= 0 {
		return fmt.Errorf("%w (id %#02x)", ErrNotInBag, escapeRopeItem)
	}
	before := mem.U8(sym.CurMap)
	if err := useOverworldKeyItem(m, escapeRopeItem, func(mm *state.Mem) bool {
		return mm.U8(sym.CurMap) != before
	}); err != nil {
		return fmt.Errorf("Escape Rope: %w", err)
	}
	return waitFastTravelArrival(m, landing)
}

// maybeUseFastTravel prices legal shortcuts against the ordinary semantic
// route. false,nil means normal walking won. Once an action is selected its
// failure is surfaced: menu/warp state may have changed, so pretending nothing
// happened would be less safe than letting Travel/farm replan explicitly.
func maybeUseFastTravel(m *emu.Emu, romData []byte, dest Destination) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	choice := chooseFastTravel(m, romData, &mem, dest)
	switch choice.Kind {
	case fastTravelNone:
		return false, nil
	case fastTravelFly:
		return true, useFlyTo(m, choice.Map)
	case fastTravelDig:
		return true, useDigFastTravel(m, choice.Map)
	case fastTravelEscapeRope:
		return true, useEscapeRopeFastTravel(m, choice.Map)
	default:
		return false, fmt.Errorf("unknown fast travel kind %d", choice.Kind)
	}
}
