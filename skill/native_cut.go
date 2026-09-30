package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
)

// CutNearestNativeObstacle prepares Cut, walks to the nearest reachable
// cuttable cell on the current native-map grid, faces it, and executes Cut.
// It is intentionally local: callers own the story destination and can re-run
// native routing after the live block mutation.
func CutNearestNativeObstacle(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("skill: native Cut: nil emulator")
	}
	profile, err := nativeRoutingProfileFor(m)
	if err != nil {
		return err
	}
	provider := profile.NativeMapProvider(romData)
	if provider == nil {
		return fmt.Errorf("skill: native Cut: profile returned nil native map provider")
	}
	if _, err := EnsureFieldMove(m, FieldCut); err != nil {
		return fmt.Errorf("skill: native Cut: prepare Cut: %w", err)
	}

	state := profile.DecodeOverworld(m)
	if !state.Controllable || state.InBattle || state.InDialogue {
		return fmt.Errorf("skill: native Cut: unsafe boundary on map %#04x at (%d,%d)", state.NativeMapID, state.X, state.Y)
	}
	grid, live, header, err := nativeLiveGrid(m, profile, provider, state.NativeMapID)
	if err != nil {
		return fmt.Errorf("skill: native Cut: live grid: %w", err)
	}
	blocked := nativeRuntimeBlockers(live, header, nil)
	delete(blocked, [2]int{int(state.X), int(state.Y)})

	type candidate struct {
		x, y int
		path []world.NativeStep
	}
	var best *candidate
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if !grid.Cuttable(x, y) {
				continue
			}
			path, _, err := nativeAdjacentApproach(
				grid, int(state.X), int(state.Y), x, y, blocked,
			)
			if err != nil {
				continue
			}
			if best == nil || len(path) < len(best.path) {
				best = &candidate{x: x, y: y, path: path}
			}
		}
	}
	if best == nil {
		return fmt.Errorf("skill: native Cut: no reachable cuttable obstacle on map %#04x", state.NativeMapID)
	}
	if err := walkNativePath(m, profile, best.path); err != nil {
		return fmt.Errorf("skill: native Cut: approach (%d,%d): %w", best.x, best.y, err)
	}
	if err := Face(m, uint8(best.x), uint8(best.y)); err != nil {
		return fmt.Errorf("skill: native Cut: face (%d,%d): %w", best.x, best.y, err)
	}
	field, err := fieldActionDecoderFor(m)
	if err != nil {
		return err
	}
	if !field.DecodeFieldAction(m).CuttableAhead {
		return fmt.Errorf("skill: native Cut: live target (%d,%d) is not cuttable", best.x, best.y)
	}
	if _, err := useFieldMoveWithDecoder(m, FieldCut, field); err != nil {
		return fmt.Errorf("skill: native Cut: execute at (%d,%d): %w", best.x, best.y, err)
	}
	return nil
}
