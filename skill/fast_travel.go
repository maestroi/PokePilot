package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	escapeRopeItem      uint8 = 0x1d
	digMoveID           uint8 = 0x5b
	digFieldMoveMenuID  uint8 = 7
	plateauTileset      uint8 = 23
	fastTravelWarpBudget      = 6000
)

type fastTravelKind uint8

const (
	fastTravelNone fastTravelKind = iota
	fastTravelFly
	fastTravelDig
	fastTravelEscapeRope
)

type fastTravelChoice struct {
	Kind fastTravelKind
	Map  uint8
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

func escapeTravelAllowed(mem *state.Mem) bool {
	if mem == nil {
		return false
	}
	switch mem.U8(sym.CurMapTileset) {
	case 3, 15, 16, 17, 22: // FOREST, CEMETERY, INTERIOR, CAVERN, FACILITY
		return true
	default:
		return false
	}
}

// chooseFastTravel only proposes transitions that land on the destination map
// the caller already requested. These are optional cost shortcuts, never
// obstacle prerequisites: missing moves/items simply mean ordinary routing.
func chooseFastTravel(mem *state.Mem, dest Destination) fastTravelChoice {
	if mem == nil || mem.U8(sym.CurMap) == dest.Map || !state.Controllable(mem) {
		return fastTravelChoice{}
	}

	if _, ok := flyLanding[dest.Map]; ok &&
		outsideForFly(mem) &&
		townVisited(mem, 0) && townVisited(mem, dest.Map) &&
		FieldCapabilityFor(mem, FieldFly).Usable {
		return fastTravelChoice{Kind: fastTravelFly, Map: dest.Map}
	}

	if dest.Map == mem.U8(sym.LastBlackoutMap) && escapeTravelAllowed(mem) {
		if partyMoveSlot(mem, digMoveID) >= 0 {
			return fastTravelChoice{Kind: fastTravelDig, Map: dest.Map}
		}
		if _, qty := bagEntry(mem, escapeRopeItem); qty > 0 {
			return fastTravelChoice{Kind: fastTravelEscapeRope, Map: dest.Map}
		}
	}
	return fastTravelChoice{}
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

func waitFastTravelArrival(m *emu.Emu, want uint8) error {
	if _, err := m.StepUntil(fastTravelWarpBudget, func(e *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(e, &mem)
		return mem.U8(sym.CurMap) == want && state.Controllable(&mem)
	}); err != nil {
		x, y := playerXY(m)
		return fmt.Errorf("fast travel did not reach map %#02x within %d frames; map=%#02x at (%d,%d)",
			want, fastTravelWarpBudget, m.Peek8(sym.CurMap), x, y)
	}
	return waitForPositionStable(m, positionStableBudget, positionStableFrames)
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
	if err := waitFastTravelArrival(m, destMap); err != nil {
		return err
	}
	x, y := playerXY(m)
	if x != landing.X || y != landing.Y {
		return fmt.Errorf("Fly to map %#02x landed at (%d,%d), want (%d,%d)", destMap, x, y, landing.X, landing.Y)
	}
	return nil
}

func useDigFastTravel(m *emu.Emu, destMap uint8) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	slot := partyMoveSlot(&mem, digMoveID)
	if slot < 0 || !escapeTravelAllowed(&mem) || mem.U8(sym.LastBlackoutMap) != destMap {
		return fmt.Errorf("Dig fast travel is not legal to map %#02x", destMap)
	}
	if err := openPartyFieldMove(m, slot, digFieldMoveMenuID); err != nil {
		return fmt.Errorf("Dig: %w", err)
	}
	return waitFastTravelArrival(m, destMap)
}

func useEscapeRopeFastTravel(m *emu.Emu, destMap uint8) error {
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
	return waitFastTravelArrival(m, destMap)
}

// maybeUseFastTravel performs an optional direct-map shortcut. false,nil means
// ordinary pathing should proceed. Once an action is selected its failure is
// surfaced: menu/warp state may have changed, so silently pretending nothing
// happened would be less safe than letting Travel/farm replan explicitly.
func maybeUseFastTravel(m *emu.Emu, dest Destination) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	choice := chooseFastTravel(&mem, dest)
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
