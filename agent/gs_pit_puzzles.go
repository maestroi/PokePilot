package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// Retail B1F / Gym 2F stonetable associates object slots with a *specific*
// pit warp. The first map object is slot 2; Gym has two trainers before its
// boulder object slots. A nearby but different pit would not be correct.
var gsIcePathPitTargets = map[int]world.Point{
	2: {X: 11, Y: 2},
	3: {X: 4, Y: 7},
	4: {X: 5, Y: 12},
	5: {X: 12, Y: 13},
}

var gsBlackthornPitTargets = map[int]world.Point{
	4: {X: 8, Y: 3},
	5: {X: 2, Y: 5},
	6: {X: 8, Y: 7},
}

// gsSolveRetailSinkPuzzle executes a single native Strength push per plan.
// Re-observe the object table, map collision and cartridge event after every
// push; scripts such as "The boulder fell through!" need their dialogue
// settled before another action. No RAM object/event changes are fabricated.
func gsSolveRetailSinkPuzzle(m *emu.Emu, romData []byte, profile *gsprofile.Profile,
	mapName string, stagingX, stagingY uint8, sinks map[int]world.Point,
	label string, complete func() bool,
) error {
	if _, err := skill.EnsureFieldMove(m, skill.FieldStrength); err != nil {
		return fmt.Errorf("gen2 %s: prepare Strength: %w", label, err)
	}
	mapID, err := gsOpeningMapID(mapName)
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(mapID, stagingX, stagingY)); err != nil {
		return fmt.Errorf("gen2 %s: reach boulder chamber: %w", label, err)
	}
	for push := 0; push < 256; push++ {
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
				return fmt.Errorf("gen2 %s: settle previous boulder event: %w", label, err)
			}
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if profile.DecodeOverworld(m).NativeMapID != mapID {
			return fmt.Errorf("%w: %s left owned boulder map", errGSSecondBadgeUnexpectedState, label)
		}
		moved, err := skill.PushNativeStrengthTowardSink(m, romData, sinks)
		if err != nil {
			return fmt.Errorf("gen2 %s: verified Strength push %d: %w", label, push+1, err)
		}
		if !moved {
			return fmt.Errorf("%w: %s cannot plan a legal Strength push into its remaining matching pits",
				errGSSecondBadgeUnexpectedState, label)
		}
	}
	return fmt.Errorf("%w: %s exceeded 256 boulder pushes", errGSSecondBadgeStalled, label)
}

func executeGSIcePathBoulders(m *emu.Emu, romData []byte) error {
	const label = "ice-path:b1f-four-pits"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressIcePathBouldersDropped,
		gsprofile.ProgressIcePathWaterfallAcquired, label)
	if err != nil || done {
		return err
	}
	// (3,15) is a warp back to 1F; (3,14) is the adjacent staging tile.
	return gsSolveRetailSinkPuzzle(m, romData, profile, "ICE_PATH_B1F", 3, 14,
		gsIcePathPitTargets, label,
		func() bool { return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressIcePathBouldersDropped) })
}

func executeGSBlackthornGymBoulders(m *emu.Emu, romData []byte) error {
	const label = "blackthorn:gym2f-three-pits"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressBlackthornGymBouldersDropped,
		gsprofile.ProgressIcePathBouldersDropped, label)
	if err != nil || done {
		return err
	}
	if err := gsCoastalHeal(m, romData, profile, "BLACKTHORN_POKECENTER_1F"); err != nil {
		return fmt.Errorf("gen2 Blackthorn Gym: reach city and heal: %w", err)
	}
	// (1,7) warps back to 1F; (1,6) is stable staging on 2F.
	return gsSolveRetailSinkPuzzle(m, romData, profile, "BLACKTHORN_GYM_2F", 1, 6,
		gsBlackthornPitTargets, label,
		func() bool {
			return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressBlackthornGymBouldersDropped)
		})
}
