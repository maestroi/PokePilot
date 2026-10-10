package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// nativeAscendingWaterfall recognizes an upward entry into a real waterfall
// from the adapter's semantic collision grid. Downstream (downward) travel
// must stay ordinary water traversal; the ROM does not require HM07 to descend.
func nativeAscendingWaterfall(grid *world.NativeGrid, x, y int, step world.NativeStep) bool {
	return grid != nil && grid.Traversal == world.TraversalWater &&
		step.DX == 0 && step.DY == -1 &&
		grid.Obstacle(x, y-1) == worldmodel.ObstacleWaterfall
}

// walkNativePathWithWaterfall shares native walking's step and interruption
// contract but intercepts an upward Waterfall before the controller steps into
// it. A successful climb changes the position nonlocally: return climbed so
// callers discard the remainder of their planned route and re-observe.
func walkNativePathWithWaterfall(
	m *emu.Emu,
	profile nativeRoutingProfile,
	grid *world.NativeGrid,
	romData []byte,
	path []world.NativeStep,
) (climbed bool, err error) {
	start := profile.DecodeOverworld(m)
	x, y := int(start.X), int(start.Y)
	for _, step := range path {
		if nativeAscendingWaterfall(grid, x, y, step) {
			field, ok := any(profile).(game.FieldMoveDecoder)
			if !ok {
				return false, fmt.Errorf("%w: Waterfall is not exposed by this game profile", ErrFieldMovePrerequisite)
			}
			capability, err := fieldMoveCapabilityWithProfile(field, m, romData, FieldWaterfall)
			if err != nil {
				return false, err
			}
			if !capability.BadgeOwned ||
				!(capability.Usable || (capability.MachineOwned && capability.Preparable)) {
				return false, fmt.Errorf("%w: Waterfall needs the Rising Badge and an HM07 carrier", ErrFieldMovePrerequisite)
			}
			if err := Face(m, uint8(x), uint8(y-1)); err != nil {
				if errors.Is(err, ErrBattle) {
					return false, ErrBattle
				}
				return false, fmt.Errorf("skill: native waterfall: face north: %w", err)
			}
			if _, err := UseFieldMove(m, FieldWaterfall); err != nil {
				return false, fmt.Errorf("skill: native waterfall: ascend: %w", err)
			}
			after := profile.DecodeOverworld(m)
			if !after.Controllable || after.NativeMapID != start.NativeMapID ||
				after.X != uint8(x) || after.Y >= uint8(y) {
				return false, fmt.Errorf("skill: native waterfall: climbed without verified northward arrival: %w", ErrNavigationStalled)
			}
			return true, nil
		}

		if err := walkNativePath(m, profile, []world.NativeStep{step}); err != nil {
			return false, err
		}
		x += step.DX
		y += step.DY
	}
	return false, nil
}
