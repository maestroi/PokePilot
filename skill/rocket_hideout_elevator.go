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
	rocketHideoutElevatorMap uint8 = 0xCB
	liftKeyItem              uint8 = 0x4A

	rocketLiftKeyRocketX uint8 = 11
	rocketLiftKeyRocketY uint8 = 2
	rocketLiftKeyX       uint8 = 10
	rocketLiftKeyY       uint8 = 2
)

func rocketBagHas(m *emu.Emu, item uint8) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	_, count := bagEntry(&mem, item)
	return count > 0
}

// rocketB4FGuardsReachable distinguishes B4F's two live regions. While the
// boss door is locked, the stair arrival at (19,11) is disconnected from the
// guards/elevator region. The live block buffer is authoritative here: after
// both guards are beaten and B4F is reloaded, the same query sees the opened
// door and the regions become connected.
func rocketB4FGuardsReachable(m *emu.Emu, romData []byte) (bool, error) {
	if got := m.Peek8(sym.CurMap); got != rocketHideoutB4FMap {
		return false, fmt.Errorf("skill: RocketHideout: guard-side probe on map %#04x, want B4F %#04x", got, rocketHideoutB4FMap)
	}
	h, err := rom.ParseMap(romData, rocketHideoutB4FMap)
	if err != nil {
		return false, err
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return false, err
	}
	x, y := playerXY(m)
	_, _, err = world.FindPathAdjacent(grid, int(x), int(y), int(rocketGuard1X), int(rocketGuard1Y), nil)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, world.ErrNoPath) {
		return false, nil
	}
	return false, err
}

func rocketB4FBossRoomReachable(m *emu.Emu, romData []byte) (bool, error) {
	if got := m.Peek8(sym.CurMap); got != rocketHideoutB4FMap {
		return false, fmt.Errorf("skill: RocketHideout: boss-room probe on map %#04x, want B4F %#04x", got, rocketHideoutB4FMap)
	}
	h, err := rom.ParseMap(romData, rocketHideoutB4FMap)
	if err != nil {
		return false, err
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return false, err
	}
	x, y := playerXY(m)
	// giovanniStand (25,4) is never itself walkable; route adjacent to
	// Giovanni's real tile instead (see walkRocketBossDoor).
	_, _, err = world.FindPathAdjacent(grid, int(x), int(y), int(giovanniX), int(giovanniY), spriteBlockers(m))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, world.ErrNoPath) {
		return false, nil
	}
	return false, err
}

// acquireRocketLiftKey completes the mandatory stair-side B4F branch. The
// third Rocket reveals the Lift Key as a ground object only after his
// after-battle script runs, so defeating him and picking up the item are one
// semantic prerequisite for entering the elevator route to the two door
// guards.
func acquireRocketLiftKey(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if rocketBagHas(m, liftKeyItem) {
		return nil
	}
	if got := m.Peek8(sym.CurMap); got != rocketHideoutB4FMap {
		return fmt.Errorf("skill: RocketHideout: Lift Key requested on map %#04x, want B4F %#04x", got, rocketHideoutB4FMap)
	}
	if err := fightStoryTrainerAt(m, romData, rocketLiftKeyRocketX, rocketLiftKeyRocketY, "B4F Lift Key Rocket", policy); err != nil {
		return err
	}
	if err := Pickup(m, romData, rocketLiftKeyX, rocketLiftKeyY, liftKeyItem, policy); err != nil {
		return fmt.Errorf("skill: RocketHideout: collect Lift Key: %w", err)
	}
	if !rocketBagHas(m, liftKeyItem) {
		return fmt.Errorf("skill: RocketHideout: Lift Key missing from bag after pickup")
	}
	return nil
}

// reachRocketB2FElevatorFloor returns from the stair-side B4F branch only as
// far as B2F. B2F has its own elevator, so climbing one floor farther to B1F
// is unnecessary and can cross B1F's runtime-replaced door using stale ROM
// collision. A B1F resume descends through the stair on the same side of that
// door, preserving resumability without depending on the mutable block.
func reachRocketB2FElevatorFloor(m *emu.Emu, romData []byte, policy MovePolicy) error {
	for {
		switch m.Peek8(sym.CurMap) {
		case rocketHideoutB4FMap:
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB4FMap, To: rocketHideoutB3FMap, WarpX: 19, WarpY: 10}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("skill: RocketHideout: B4F -> B3F after Lift Key: %w", err)
			}
		case rocketHideoutB3FMap:
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB3FMap, To: rocketHideoutB2FMap, WarpX: 25, WarpY: 6}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("skill: RocketHideout: reverse B3F forced-movement floor: %w", err)
			}
		case rocketHideoutB2FMap:
			return nil
		case rocketHideoutB1FMap:
			_, y := playerXY(m)
			warpX, warpY := uint8(23), uint8(2)
			if y >= 18 {
				warpX, warpY = 21, 24
			}
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB1FMap, To: rocketHideoutB2FMap, WarpX: warpX, WarpY: warpY}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("skill: RocketHideout: B1F -> B2F for elevator: %w", err)
			}
		default:
			return fmt.Errorf("skill: RocketHideout: cannot reach B2F elevator floor from map %#04x", m.Peek8(sym.CurMap))
		}
	}
}

func enterRocketElevatorFromB2F(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) == rocketHideoutElevatorMap {
		return nil
	}
	if got := m.Peek8(sym.CurMap); got != rocketHideoutB2FMap {
		return fmt.Errorf("skill: RocketHideout: elevator entrance requested on map %#04x, want B2F %#04x", got, rocketHideoutB2FMap)
	}
	edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB2FMap, To: rocketHideoutElevatorMap, WarpX: 24, WarpY: 19}
	if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
		return fmt.Errorf("skill: RocketHideout: enter B2F elevator: %w", err)
	}
	if got := m.Peek8(sym.CurMap); got != rocketHideoutElevatorMap {
		return fmt.Errorf("skill: RocketHideout: B2F elevator entrance reached map %#04x, want %#04x", got, rocketHideoutElevatorMap)
	}
	return nil
}

func rideRocketElevatorToB4F(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) == rocketHideoutB4FMap {
		return nil
	}
	if got := m.Peek8(sym.CurMap); got != rocketHideoutElevatorMap {
		return fmt.Errorf("skill: RocketHideout: B4F elevator ride requested on map %#04x, want elevator %#04x", got, rocketHideoutElevatorMap)
	}
	if !rocketBagHas(m, liftKeyItem) {
		return fmt.Errorf("skill: RocketHideout: cannot operate elevator without Lift Key")
	}
	edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutElevatorMap, To: rocketHideoutB4FMap, WarpX: 2, WarpY: 1}
	if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
		return fmt.Errorf("skill: RocketHideout: ride elevator to B4F: %w", err)
	}
	if got := m.Peek8(sym.CurMap); got != rocketHideoutB4FMap {
		return fmt.Errorf("skill: RocketHideout: elevator reached map %#04x, want B4F %#04x", got, rocketHideoutB4FMap)
	}
	return nil
}

func reachRocketGuardSide(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if !rocketBagHas(m, liftKeyItem) {
		return fmt.Errorf("skill: RocketHideout: guard side requires Lift Key")
	}

	switch m.Peek8(sym.CurMap) {
	case rocketHideoutB4FMap:
		reachable, err := rocketB4FGuardsReachable(m, romData)
		if err != nil {
			return fmt.Errorf("skill: RocketHideout: inspect B4F guard side: %w", err)
		}
		if reachable {
			return nil
		}
		if err := reachRocketB2FElevatorFloor(m, romData, policy); err != nil {
			return err
		}
		if err := enterRocketElevatorFromB2F(m, romData, policy); err != nil {
			return err
		}
		return rideRocketElevatorToB4F(m, romData, policy)
	case rocketHideoutB3FMap, rocketHideoutB2FMap, rocketHideoutB1FMap:
		if err := reachRocketB2FElevatorFloor(m, romData, policy); err != nil {
			return err
		}
		if err := enterRocketElevatorFromB2F(m, romData, policy); err != nil {
			return err
		}
		return rideRocketElevatorToB4F(m, romData, policy)
	case rocketHideoutElevatorMap:
		return rideRocketElevatorToB4F(m, romData, policy)
	default:
		return fmt.Errorf("skill: RocketHideout: cannot reach guard side from map %#04x", m.Peek8(sym.CurMap))
	}
}

// reloadRocketB4FViaElevator ensures the live B4F boss door callback has been
// applied after both guards are beaten. Some battle/script paths apply the
// callback immediately without a map reload; in that case the live map is
// already authoritative and leaving through the elevator is both unnecessary
// and can fail from the guard-side landing. Only reload when the boss room is
// still disconnected in the live block buffer.
func reloadRocketB4FViaElevator(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if got := m.Peek8(sym.CurMap); got != rocketHideoutB4FMap {
		return fmt.Errorf("skill: RocketHideout: B4F reload requested on map %#04x", got)
	}
	open, err := rocketB4FBossRoomReachable(m, romData)
	if err != nil {
		return fmt.Errorf("skill: RocketHideout: inspect live B4F boss door: %w", err)
	}
	if open {
		return nil
	}
	edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB4FMap, To: rocketHideoutElevatorMap, WarpX: 24, WarpY: 15}
	if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
		return fmt.Errorf("skill: RocketHideout: leave B4F through elevator: %w", err)
	}
	return rideRocketElevatorToB4F(m, romData, policy)
}
