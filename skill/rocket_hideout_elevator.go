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

// ascendRocketHideoutToB1F walks the stair-side route in reverse after the
// Lift Key is collected. B2F/B3F still need spinner-aware routing in reverse;
// ordinary graph walking cannot safely infer the forced arrow movement.
func ascendRocketHideoutToB1F(m *emu.Emu, romData []byte, policy MovePolicy) error {
	for {
		switch m.Peek8(sym.CurMap) {
		case rocketHideoutB4FMap:
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB4FMap, To: rocketHideoutB3FMap, WarpX: 19, WarpY: 10}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("skill: RocketHideout: B4F -> B3F after Lift Key: %w", err)
			}
		case rocketHideoutB3FMap:
			if err := travelRocketWarp(m, policy, func() error {
				return walkRocketSpinnerWarp(m, romData, rocketHideoutB3FMap, rocketHideoutB2FMap, 25, 6)
			}); err != nil {
				return fmt.Errorf("skill: RocketHideout: reverse B3F spinner floor: %w", err)
			}
		case rocketHideoutB2FMap:
			if err := travelRocketWarp(m, policy, func() error {
				return walkRocketSpinnerWarp(m, romData, rocketHideoutB2FMap, rocketHideoutB1FMap, 27, 8)
			}); err != nil {
				return fmt.Errorf("skill: RocketHideout: reverse B2F spinner floor: %w", err)
			}
		case rocketHideoutB1FMap:
			return nil
		default:
			return fmt.Errorf("skill: RocketHideout: cannot return to B1F from map %#04x", m.Peek8(sym.CurMap))
		}
	}
}

func enterRocketElevatorFromB1F(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) == rocketHideoutElevatorMap {
		return nil
	}
	if got := m.Peek8(sym.CurMap); got != rocketHideoutB1FMap {
		return fmt.Errorf("skill: RocketHideout: elevator entrance requested on map %#04x, want B1F %#04x", got, rocketHideoutB1FMap)
	}
	edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB1FMap, To: rocketHideoutElevatorMap, WarpX: 24, WarpY: 19}
	if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
		return fmt.Errorf("skill: RocketHideout: enter B1F elevator: %w", err)
	}
	if got := m.Peek8(sym.CurMap); got != rocketHideoutElevatorMap {
		return fmt.Errorf("skill: RocketHideout: B1F elevator entrance reached map %#04x, want %#04x", got, rocketHideoutElevatorMap)
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

// reachRocketGuardSide resumes from any normal post-key Hideout location and
// guarantees B4F is loaded on the elevator/guard side. A B4F resume uses live
// connectivity rather than coordinates, so an already-open boss door remains
// valid and does not force an unnecessary stair backtrack.
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
		if err := ascendRocketHideoutToB1F(m, romData, policy); err != nil {
			return err
		}
		if err := enterRocketElevatorFromB1F(m, romData, policy); err != nil {
			return err
		}
		return rideRocketElevatorToB4F(m, romData, policy)
	case rocketHideoutB3FMap, rocketHideoutB2FMap, rocketHideoutB1FMap:
		if err := ascendRocketHideoutToB1F(m, romData, policy); err != nil {
			return err
		}
		if err := enterRocketElevatorFromB1F(m, romData, policy); err != nil {
			return err
		}
		return rideRocketElevatorToB4F(m, romData, policy)
	case rocketHideoutElevatorMap:
		return rideRocketElevatorToB4F(m, romData, policy)
	default:
		return fmt.Errorf("skill: RocketHideout: cannot reach guard side from map %#04x", m.Peek8(sym.CurMap))
	}
}

// reloadRocketB4FViaElevator applies RocketHideoutB4FDoorCallbackScript after
// both guard flags are set. The locked door prevents the guard side from
// reaching the B3F stair, so the elevator is not merely an optimization: it is
// the legal exit/re-entry path that causes the ROM to replace block $2d with
// the open floor block.
func reloadRocketB4FViaElevator(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if got := m.Peek8(sym.CurMap); got != rocketHideoutB4FMap {
		return fmt.Errorf("skill: RocketHideout: B4F reload requested on map %#04x", got)
	}
	edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB4FMap, To: rocketHideoutElevatorMap, WarpX: 24, WarpY: 15}
	if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
		return fmt.Errorf("skill: RocketHideout: leave B4F through elevator: %w", err)
	}
	return rideRocketElevatorToB4F(m, romData, policy)
}
