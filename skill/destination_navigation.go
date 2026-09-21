package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
)

// resolveLocalDestination converts a semantic goal on the current map into an
// exact standing tile. satisfied=true means the current position already
// fulfills the semantic destination and no local walking is needed.
func resolveLocalDestination(m *emu.Emu, romData []byte, dest Destination) (exact Destination, satisfied bool, err error) {
	cur := m.Peek8(sym.CurMap)
	if cur != dest.Map {
		return Destination{}, false, fmt.Errorf("semantic destination map %02x while current map is %02x", dest.Map, cur)
	}
	x, y := playerXY(m)
	switch dest.Kind {
	case DestinationMap:
		return Destination{Map: cur, X: x, Y: y}, true, nil
	case DestinationArea:
		if dest.Reached(cur, x, y) {
			return Destination{Map: cur, X: x, Y: y}, true, nil
		}
		exact, err := cheapestAreaDestination(m, romData, dest)
		return exact, false, err
	case DestinationInteraction:
		if _, ok := directionTo(x, y, dest.X, dest.Y); ok {
			return Destination{Map: cur, X: x, Y: y}, true, nil
		}
		if beside, ok, counterErr := counterBeside(m, romData, dest.X, dest.Y); counterErr != nil {
			return Destination{}, false, counterErr
		} else if ok {
			return beside, false, nil
		}
		beside, ok, adjacentErr := besideDestination(m, romData, dest.X, dest.Y)
		if adjacentErr != nil {
			return Destination{}, false, adjacentErr
		}
		if !ok {
			return Destination{Map: cur, X: x, Y: y}, true, nil
		}
		return beside, false, nil
	default:
		if dest.Reached(cur, x, y) {
			return dest, true, nil
		}
		return Destination{Map: dest.Map, X: dest.X, Y: dest.Y}, false, nil
	}
}

func interactionDestinationForRole(romData []byte, mapID uint8, role rom.ObjectInteractionRole) (Destination, bool, error) {
	actors, err := rom.SpecialInteractionActors(romData, mapID)
	if err != nil {
		return Destination{}, false, err
	}
	for _, actor := range actors {
		if actor.Role == role {
			return InteractionDestination(mapID, actor.X, actor.Y), true, nil
		}
	}
	return Destination{}, false, nil
}

// cheapestAreaDestination chooses the least-cost currently executable tile in
// an area. It uses the same Cut/Surf-aware local planner as GoTo, so "area"
// means a reachable region rather than merely the closest coordinate by
// Manhattan distance.
func cheapestAreaDestination(m *emu.Emu, romData []byte, dest Destination) (Destination, error) {
	h, err := rom.ParseMap(romData, dest.Map)
	if err != nil {
		return Destination{}, fmt.Errorf("parse area map %02x: %w", dest.Map, err)
	}
	sx, sy := playerXY(m)
	blocked := warpAvoidance(h, int(sx), int(sy), spriteBlockers(m))

	var (
		best     Destination
		bestCost fieldPathCost
		found    bool
	)
	for y := int(dest.Area.MinY); y <= int(dest.Area.MaxY); y++ {
		for x := int(dest.Area.MinX); x <= int(dest.Area.MaxX); x++ {
			if x < 0 || y < 0 || x > 0xff || y > 0xff {
				continue
			}
			candidate := Destination{Map: dest.Map, X: uint8(x), Y: uint8(y)}
			_, cost, planErr := currentFieldPathPlanWithCost(m, romData, h, candidate, blocked)
			if planErr != nil {
				continue
			}
			if !found || fieldPathCostLessForTravel(m, cost, bestCost) {
				best, bestCost, found = candidate, cost, true
			}
		}
	}
	if !found {
		return Destination{}, fmt.Errorf("no reachable tile in area [%d,%d]-[%d,%d] on map %02x",
			dest.Area.MinX, dest.Area.MinY, dest.Area.MaxX, dest.Area.MaxY, dest.Map)
	}
	return best, nil
}

func fieldPathCostLessForTravel(m *emu.Emu, a, b fieldPathCost) bool {
	if travelCostPolicyFor(m) == TravelCostFastest {
		if a.weighted != b.weighted {
			return a.weighted < b.weighted
		}
	}
	if a.actions != b.actions {
		return a.actions < b.actions
	}
	return a.moves < b.moves
}
