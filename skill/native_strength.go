package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

// planNativeStrengthBridge asks the portable push solver whether the
// observed movable boulders can connect the current component to a goal.
// Fixed objects, static warp triggers and unrelated sprites are immutable;
// only profile-confirmed pushable objects may enter the search state.
func planNativeStrengthBridge(
	grid *world.NativeGrid,
	live game.LiveTopologyState,
	player world.Point,
	blocked map[[2]int]bool,
	goals [][2]int,
) (world.NativePush, error) {
	var movables []world.Movable
	fixed := make(map[[2]int]bool, len(blocked))
	for k, yes := range blocked {
		if yes {
			fixed[k] = true
		}
	}
	for _, object := range live.LiveObjects {
		if !object.Pushable {
			continue
		}
		movables = append(movables, world.Movable{
			ID: object.Slot, Pos: world.Point{X: object.X, Y: object.Y},
		})
		delete(fixed, [2]int{object.X, object.Y})
	}
	goalPoints := make([]world.Point, 0, len(goals))
	for _, p := range goals {
		goalPoints = append(goalPoints, world.Point{X: p[0], Y: p[1]})
	}
	return world.FindNativeStrengthPush(grid, player, movables, fixed, goalPoints, 1500)
}

// nativeStrengthBridge is a single verified state transition, never a chain
// of speculative pushes. The router calls it after an ordinary no-path and
// rebuilds the grid and live object positions after every successful push.
func nativeStrengthBridge(
	m *emu.Emu,
	profile nativeRoutingProfile,
	romData []byte,
	grid *world.NativeGrid,
	live game.LiveTopologyState,
	player world.Point,
	blocked map[[2]int]bool,
	goals [][2]int,
) (bool, error) {
	if live.Traversal != game.TraversalLand {
		return false, nil
	}
	push, err := planNativeStrengthBridge(grid, live, player, blocked, goals)
	if errors.Is(err, world.ErrNoPath) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	decoder, ok := any(profile).(game.FieldMoveDecoder)
	if !ok {
		return false, nil
	}
	action, ok := any(profile).(game.FieldActionDecoder)
	if !ok {
		return false, nil
	}
	capability, err := fieldMoveCapabilityWithProfile(decoder, m, romData, FieldStrength)
	if errors.Is(err, ErrFieldMovePrerequisite) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !capability.BadgeOwned || !(capability.Usable || (capability.MachineOwned && capability.Preparable)) {
		return false, nil
	}
	if err := walkNativePath(m, profile, push.Walk); err != nil {
		var blockedStep *ErrBlocked
		if errors.As(err, &blockedStep) {
			m.StepFrames(npcWaitFrames)
			return true, nil // Live NPC changed; retry from a new snapshot.
		}
		return false, err
	}
	before := profile.DecodeOverworld(m)
	if !before.Controllable || before.NativeMapID != grid.MapID ||
		int(before.X) != push.Stand.X || int(before.Y) != push.Stand.Y {
		return false, fmt.Errorf("skill: native Strength: approach to slot %d was not verified: %w", push.MovableID, ErrNavigationStalled)
	}
	fieldState := action.DecodeFieldAction(m)
	if !fieldState.StrengthActive {
		if _, err := UseFieldMove(m, FieldStrength); err != nil {
			return false, fmt.Errorf("skill: native Strength: activate: %w", err)
		}
		if !action.DecodeFieldAction(m).StrengthActive {
			return false, fmt.Errorf("skill: native Strength: activation did not persist: %w", ErrNavigationStalled)
		}
	}
	if err := walkNativePath(m, profile, []world.NativeStep{push.Direction}); err != nil {
		return false, fmt.Errorf("skill: native Strength: push slot %d: %w", push.MovableID, err)
	}
	after := profile.DecodeOverworld(m)
	if !after.Controllable || after.NativeMapID != grid.MapID ||
		int(after.X) != push.From.X || int(after.Y) != push.From.Y {
		return false, fmt.Errorf("skill: native Strength: player did not enter boulder's old tile: %w", ErrNavigationStalled)
	}
	next, err := profile.DecodeLiveTopology(m)
	if err != nil {
		return false, err
	}
	for _, object := range next.LiveObjects {
		if object.Slot == push.MovableID {
			if !object.Pushable || object.X != push.To.X || object.Y != push.To.Y {
				return false, fmt.Errorf("skill: native Strength: boulder slot %d did not move to (%d,%d): %w",
					push.MovableID, push.To.X, push.To.Y, ErrNavigationStalled)
			}
			return true, nil
		}
	}
	return false, fmt.Errorf("skill: native Strength: boulder slot %d disappeared after push (terminal hole needs an event postcondition): %w",
		push.MovableID, ErrNavigationStalled)
}

