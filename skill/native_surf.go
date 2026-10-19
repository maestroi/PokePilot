package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// nativeSurfPlan is the land-only prefix of a route which becomes possible
// after the player boards Surf. The first water tile is a *facing target*, not
// a step to send before the field move has made the water traversable.
type nativeSurfPlan struct {
	approach []world.NativeStep
	shore    game.MapPoint
	water    game.MapPoint
}

// planNativeSurfApproach asks whether Surf, rather than a detour or an object
// interaction, is what separates the player's current component from a goal.
// Both grids describe the SAME live map: only the adapter's traversal mode
// changes. It never uses a water-only route to walk to the shoreline.
//
// FindNativePath honors Gen-II one-way walls, ledges, warps, and live sprite
// occupancy, so the first water tile is reachable from a verified land path
// and is directly adjacent to the final shore position.
//
// Destination tiles are allowed even when blocked: nativeRuntimeBlockers marks
// every non-inert warp occupied so land routes cannot step on them by accident,
// but a Surf goal is often itself a warp carpet (Route 42's west gate). Matching
// nativeWarpApproach, the target must stay reachable while other warps remain
// sealed so water can still detour around cave doors on the land corridor.
func planNativeSurfApproach(land, water *world.NativeGrid, sx, sy int, targets [][2]int, blocked map[[2]int]bool) (nativeSurfPlan, bool) {
	if land == nil || water == nil || land.MapID != water.MapID ||
		land.Width != water.Width || land.Height != water.Height ||
		!land.InBounds(sx, sy) {
		return nativeSurfPlan{}, false
	}

	bestCost := int(^uint(0) >> 1)
	var best nativeSurfPlan
	found := false
	for _, target := range targets {
		// Allow the Surf goal itself; keep every other blocker (sibling warps,
		// sprites) so the water route cannot cheat through a cave door.
		waterBlocked := nativeAllowTarget(blocked, target)
		waterPath, err := world.FindNativePath(water, sx, sy, target[0], target[1], waterBlocked)
		if err != nil {
			continue
		}
		x, y := sx, sy
		for _, step := range waterPath {
			nx, ny := x+step.DX, y+step.DY
			if !land.Walkable(nx, ny) && water.Walkable(nx, ny) &&
				absNativeStep(step) == 1 && water.Passable(x, y, nx, ny) {
				shorePath, shoreErr := world.FindNativePath(land, sx, sy, x, y, blocked)
				if shoreErr == nil && len(shorePath)+len(waterPath) < bestCost {
					bestCost = len(shorePath) + len(waterPath)
					best = nativeSurfPlan{
						approach: shorePath,
						shore:    game.MapPoint{X: x, Y: y},
						water:    game.MapPoint{X: nx, Y: ny},
					}
					found = true
				}
				break
			}
			x, y = nx, ny
		}
	}
	return best, found
}

// nativeWaterObstacleStands lists, per goal that water reaches only past one
// usable obstacle (a whirlpool), the tile from which that obstacle is cleared.
// Surfing there is progress: once afloat, the ordinary obstacle bridge clears
// it on the water grid and the walk replans.
func nativeWaterObstacleStands(
	water *world.NativeGrid,
	sx, sy int,
	goals [][2]int,
	blocked map[[2]int]bool,
	usable func(worldmodel.NativeObstacle) bool,
) [][2]int {
	var stands [][2]int
	for _, goal := range goals {
		plan, err := world.FindNativeObstacleApproach(water, sx, sy, goal[0], goal[1], nativeAllowTarget(blocked, goal), usable)
		if err != nil {
			continue
		}
		x, y := sx, sy
		for _, step := range plan.Approach {
			x, y = x+step.DX, y+step.DY
		}
		stands = append(stands, [2]int{x, y})
	}
	return stands
}

func absNativeStep(s world.NativeStep) int {
	dx, dy := s.DX, s.DY
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

// nativeSurfBridge transitions from a land component to water traversal only
// when a reachable water-mode route crosses a real water tile. It is shared by
// same-map destinations, warp approaches, and connection-edge approaches.
// No source map, shoreline coordinate or ROM collision byte is in this layer.
func nativeSurfBridge(
	m *emu.Emu,
	profile nativeRoutingProfile,
	provider worldmodel.NativeGridProvider,
	romData []byte,
	mapID uint16,
	land *world.NativeGrid,
	live game.LiveTopologyState,
	sx, sy int,
	blocked map[[2]int]bool,
	targets func(*world.NativeGrid) ([][2]int, error),
) (bool, error) {
	if live.Traversal != game.TraversalLand {
		return false, nil // Already surfing; another field action cannot help.
	}
	spec, err := provider.Grid(mapID, live.Blocks, worldmodel.TraversalWater)
	if err != nil {
		return false, fmt.Errorf("skill: native surf: water collision for map %#04x: %w", mapID, err)
	}
	water, err := world.NativeGridFromSpec(spec)
	if err != nil {
		return false, fmt.Errorf("skill: native surf: invalid water grid for map %#04x: %w", mapID, err)
	}
	goals, err := targets(water)
	if err != nil {
		return false, err
	}
	plan, ok := planNativeSurfApproach(land, water, sx, sy, goals, blocked)
	if !ok {
		usable, usableErr := nativeObstacleUsable(profile, m, romData)
		if usableErr != nil {
			return false, fmt.Errorf("skill: native surf: decode field-move capability: %w", usableErr)
		}
		plan, ok = planNativeSurfApproach(land, water, sx, sy,
			nativeWaterObstacleStands(water, sx, sy, goals, blocked, usable), blocked)
	}
	if !ok {
		return false, nil
	}
	field, ok := any(profile).(game.FieldMoveDecoder)
	if !ok {
		return false, nil
	}
	capability, err := fieldMoveCapabilityWithProfile(field, m, romData, FieldSurf)
	if err != nil {
		if errors.Is(err, ErrFieldMovePrerequisite) {
			return false, nil
		}
		return false, err
	}
	if !capability.BadgeOwned || !(capability.Usable || (capability.MachineOwned && capability.Preparable)) {
		return false, nil
	}

	if err := walkNativePath(m, profile, plan.approach); err != nil {
		var blockedStep *ErrBlocked
		if errors.As(err, &blockedStep) {
			// Live NPC moved after planning; retry from a new snapshot.
			m.StepFrames(npcWaitFrames)
			return true, nil
		}
		return false, err
	}
	state := profile.DecodeOverworld(m)
	if state.NativeMapID != mapID || !state.Controllable ||
		int(state.X) != plan.shore.X || int(state.Y) != plan.shore.Y {
		return false, fmt.Errorf("skill: native surf: shore approach did not settle at (%d,%d): %w",
			plan.shore.X, plan.shore.Y, ErrNavigationStalled)
	}
	if err := Face(m, uint8(plan.water.X), uint8(plan.water.Y)); err != nil {
		if errors.Is(err, ErrBattle) {
			return false, ErrBattle
		}
		return false, fmt.Errorf("skill: native surf: face water at (%d,%d): %w", plan.water.X, plan.water.Y, err)
	}
	result, err := UseFieldMove(m, FieldSurf)
	if err != nil {
		return false, fmt.Errorf("skill: native surf: enter water: %w", err)
	}
	after, err := profile.DecodeLiveTopology(m)
	if err != nil {
		return false, err
	}
	if !result.Surfing || after.NativeMapID != mapID || after.Traversal != game.TraversalWater ||
		!profile.DecodeOverworld(m).Controllable {
		return false, fmt.Errorf("skill: native surf: water traversal did not become active: %w", ErrNavigationStalled)
	}
	return true, nil
}
