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
	return executeNativeStrengthPush(m, profile, romData, grid, push)
}

// PushNativeStrengthTowardSink performs one live-verified move from a bounded
// solution that puts assigned puzzle boulders into real collision pits. It is
// used by map-specific story scripts, NOT as a general teleport or global
// Strength solver. The caller must re-observe each vanished boulder and its
// persistent event flag before calling again.
func PushNativeStrengthTowardSink(m *emu.Emu, romData []byte, sinks map[int]world.Point) (bool, error) {
	profile, err := nativeRoutingProfileForROM(romData)
	if err != nil {
		return false, err
	}
	if m == nil {
		return false, fmt.Errorf("skill: native sink puzzle: nil emulator")
	}
	if _, err := waitLiveMapSettled(m, profile); err != nil {
		return false, err
	}
	worldState := profile.DecodeOverworld(m)
	provider := profile.NativeMapProvider(romData)
	grid, live, header, err := nativeLiveGrid(m, profile, provider, worldState.NativeMapID)
	if err != nil {
		return false, err
	}
	if live.Traversal != game.TraversalLand {
		return false, nil
	}
	blocked := nativeRuntimeBlockers(live, header, nil)
	delete(blocked, [2]int{int(worldState.X), int(worldState.Y)})
	movables := make([]world.Movable, 0, len(live.LiveObjects))
	for _, obj := range live.LiveObjects {
		if !obj.Pushable {
			continue
		}
		movables = append(movables, world.Movable{ID: obj.Slot, Pos: world.Point{X: obj.X, Y: obj.Y}})
		delete(blocked, [2]int{obj.X, obj.Y})
	}
	push, err := world.FindNativeStrengthSinkPush(grid,
		world.Point{X: int(worldState.X), Y: int(worldState.Y)}, movables, blocked, sinks, 25000)
	if errors.Is(err, world.ErrNoPath) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("skill: native Strength pit puzzle: %w", err)
	}
	return executeNativeStrengthPush(m, profile, romData, grid, push)
}

// executeNativeStrengthPush is the single shared execution gate for ordinary
// Strength routing and boulder-to-hole puzzles. Success is a moved object or
// a disappear-on-real-COLL_PIT event; no synthetic object state is written.
func executeNativeStrengthPush(
	m *emu.Emu, profile nativeRoutingProfile, romData []byte,
	grid *world.NativeGrid, push world.NativePush,
) (bool, error) {
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
	// Strength may already be active from a prior push. Arriving on the stand
	// tile can open the boulder stdscript ("The boulders move!") before the
	// push step runs; Controllable goes false while the walk still reports
	// success. Page that box and re-check the stand instead of stalling the
	// whole pit puzzle (run-a018a6cmhnkr / triage f658a1d845ef36a5).
	if (!before.Controllable || before.InDialogue) &&
		before.NativeMapID == grid.MapID &&
		int(before.X) == push.Stand.X && int(before.Y) == push.Stand.Y {
		if rec := RecoverDialogue(m, dialogueRecoveryBudget); rec.Stop == DialogueChoiceRequired {
			return false, ErrDialogueInterrupted
		}
		before = profile.DecodeOverworld(m)
	}
	if !before.Controllable || before.NativeMapID != grid.MapID ||
		int(before.X) != push.Stand.X || int(before.Y) != push.Stand.Y {
		if !before.Controllable || before.InDialogue {
			return false, ErrDialogueInterrupted
		}
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
		var blockedStep *ErrBlocked
		if errors.As(err, &blockedStep) {
			// Live collision or an ice stop disagreed with the plan; wait and
			// replan from a fresh object snapshot instead of failing the
			// whole pit puzzle on one rejected step.
			m.StepFrames(npcWaitFrames)
			return true, nil
		}
		if errors.Is(err, ErrDialogueInterrupted) {
			// An actual pit-warp landing queues its native boulder-fall text.
			// The owning story controller settles it then checks cartridge
			// events. A disappearing boulder anywhere else is never accepted.
			if tile, ok := grid.Tile(push.To.X, push.To.Y); ok && tile == 0x60 {
				return true, nil
			}
		}
		return false, fmt.Errorf("skill: native Strength: push slot %d: %w", push.MovableID, err)
	}
	after := profile.DecodeOverworld(m)
	onFrom := after.NativeMapID == grid.MapID &&
		int(after.X) == push.From.X && int(after.Y) == push.From.Y
	// The pit-fall / boulder stdscript often opens only after step settle
	// completes, so walkNativePath returns nil while Controllable is already
	// false on From (run-19bsr1iwtwfdk378ewymcvdz2u / triage f658a1d845ef36a5
	// regression after the approach-dialogue recovery in #2796). Mirror the
	// during-push pit handoff, and page non-pit text before verifying.
	if (!after.Controllable || after.InDialogue) && onFrom {
		if tile, ok := grid.Tile(push.To.X, push.To.Y); ok && tile == 0x60 {
			return true, nil
		}
		if rec := RecoverDialogue(m, dialogueRecoveryBudget); rec.Stop == DialogueChoiceRequired {
			return false, ErrDialogueInterrupted
		}
		after = profile.DecodeOverworld(m)
		onFrom = after.NativeMapID == grid.MapID &&
			int(after.X) == push.From.X && int(after.Y) == push.From.Y
	}
	if !after.Controllable || !onFrom {
		if !after.Controllable || after.InDialogue {
			return false, ErrDialogueInterrupted
		}
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
	// Ice Path B1F and Blackthorn Gym 2F have retail pit warps: a push
	// into COLL_PIT ($60) removes the boulder and changes the live floor.
	// Only accept disappearing boulders on that exact collision (never a
	// missing sprite on ordinary ground), then replan from live geometry.
	if tile, ok := grid.Tile(push.To.X, push.To.Y); ok && tile == 0x60 {
		return true, nil
	}
	return false, fmt.Errorf("skill: native Strength: boulder slot %d disappeared on non-pit tile (%d,%d): %w",
		push.MovableID, push.To.X, push.To.Y, ErrNavigationStalled)
}
