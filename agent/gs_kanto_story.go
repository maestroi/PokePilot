package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const gsKantoStoryAttempts = 3

// gsKantoStoryTalk reuses the native story navigation and battle interruption
// driver but insists on an exact cartridge event after each interaction.
func gsKantoStoryTalk(m *emu.Emu, romData []byte, profile *gsprofile.Profile,
	progress game.ProgressID, mapName string, sx, sy, tx, ty uint8, label string,
) error {
	mapID, err := gsOpeningMapID(mapName)
	if err != nil {
		return err
	}
	done := func() bool { return gsFirstBadgeProgressComplete(profile, m, progress) }
	for attempt := 0; attempt < gsKantoStoryAttempts; attempt++ {
		if done() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
				return fmt.Errorf("gen2 %s: settle previous script: %w", label, err)
			}
		}
		// If a late Kanto objective resumes in Johto, use the real
		// PASS-powered Magnet Train instead of routing through an
		// unowned cross-region station warp.
		if err := gsKantoRestoreRegion(m, romData, profile); err != nil {
			return fmt.Errorf("gen2 %s: restore Kanto region: %w", label, err)
		}
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(mapID, sx, sy), tx, ty, label); err != nil {
			return fmt.Errorf("gen2 %s: reach interaction: %w", label, err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 %s: settle retail script: %w", label, err)
		}
		if done() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: %s never set %s", errGSSecondBadgeUnexpectedState, label, progress)
}

// executeGSKantoPowerManager talks to the actual generator manager before
// hunting for the stolen Machine Part. His script arms the Cerulean Gym
// Rocket encounter and uncovers the otherwise hidden Machine Part.
func executeGSKantoPowerManager(m *emu.Emu, romData []byte) error {
	const label = "kanto:power-plant-manager"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoManagerMet, gsprofile.ProgressKantoSoulBadgeEarned, label)
	if err != nil || done {
		return err
	}
	return gsKantoStoryTalk(m, romData, profile, gsprofile.ProgressKantoManagerMet,
		"POWER_PLANT", 14, 11, 14, 10, label)
}

// executeGSKantoRocketAtGym walks into the Cerulean Gym, where the retail
// deferred scene reveals the thief and opens Route 25's Misty-date event.
func executeGSKantoRocketAtGym(m *emu.Emu, romData []byte) error {
	const label = "kanto:cerulean-rocket"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoRocketAtGym, gsprofile.ProgressKantoManagerMet, label)
	if err != nil || done {
		return err
	}
	gym, err := gsOpeningMapID("CERULEAN_GYM")
	if err != nil {
		return err
	}
	complete := func() bool { return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressKantoRocketAtGym) }
	for attempt := 0; attempt < gsKantoStoryAttempts; attempt++ {
		if err := gsSecondBadgeGoToUntil(m, romData, profile,
			skill.ExactNativeDestination(gym, 4, 12), complete); err != nil {
			return fmt.Errorf("gen2 Cerulean Rocket: enter Gym and trigger encounter: %w", err)
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
				return err
			}
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Cerulean Gym never set the Rocket encounter event", errGSSecondBadgeUnexpectedState)
}

// executeGSKantoMachinePart inspects the actual hidden-item background tile
// (3,8), exposed by the manager's script. The receipt event proves the part
// was taken; merely standing in the Gym cannot satisfy this objective.
func executeGSKantoMachinePart(m *emu.Emu, romData []byte) error {
	const label = "kanto:machine-part"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoMachinePartFound, gsprofile.ProgressKantoRocketAtGym, label)
	if err != nil || done {
		return err
	}
	return gsKantoStoryTalk(m, romData, profile, gsprofile.ProgressKantoMachinePartFound,
		"CERULEAN_GYM", 3, 9, 3, 8, label)
}

func executeGSKantoRestorePower(m *emu.Emu, romData []byte) error {
	const label = "kanto:restore-power"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoPowerRestored, gsprofile.ProgressKantoMachinePartFound, label)
	if err != nil || done {
		return err
	}
	// PowerPlantManager.takeitem MACHINE_PART sets RETURNED_MACHINE_PART
	// and RESTORED_POWER_TO_KANTO. The profile requires both flags.
	return gsKantoStoryTalk(m, romData, profile, gsprofile.ProgressKantoPowerRestored,
		"POWER_PLANT", 14, 11, 14, 10, label)
}

// executeGSKantoMistyDate triggers Route25's real coord_event (42,6)/(42,7).
// The scene clears EVENT_TRAINERS_IN_CERULEAN_GYM, making Misty and her
// swimmers visible again. Restored power by itself is not sufficient.
func executeGSKantoMistyDate(m *emu.Emu, romData []byte) error {
	const label = "kanto:misty-date"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoMistyReturned, gsprofile.ProgressKantoPowerRestored, label)
	if err != nil || done {
		return err
	}
	mapID, err := gsOpeningMapID("ROUTE_25")
	if err != nil {
		return err
	}
	if err := gsKantoRestoreRegion(m, romData, profile); err != nil {
		return fmt.Errorf("gen2 Misty: resume in Kanto: %w", err)
	}
	complete := func() bool { return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressKantoMistyReturned) }
	for _, y := range []uint8{6, 7} {
		if err := gsSecondBadgeGoToUntil(m, romData, profile,
			skill.ExactNativeDestination(mapID, 42, y), complete); err != nil {
			return fmt.Errorf("gen2 Misty: trigger retail Route 25 date scene: %w", err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
				return err
			}
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Route 25 did not return Misty to Cerulean Gym", errGSSecondBadgeUnexpectedState)
}

// executeGSKantoInviteBlue talks to Blue on Cinnabar after Blaine. Blue's
// retail script makes his Viridian Gym object visible; the subsequent Earth
// Badge controller never assumes the empty Gym already contains its leader.
func executeGSKantoInviteBlue(m *emu.Emu, romData []byte) error {
	const label = "kanto:blue-cinnabar"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoBlueInvited, gsprofile.ProgressKantoVolcanoBadgeEarned, label)
	if err != nil || done {
		return err
	}
	return gsKantoStoryTalk(m, romData, profile, gsprofile.ProgressKantoBlueInvited,
		"CINNABAR_ISLAND", 9, 7, 9, 6, label)
}
