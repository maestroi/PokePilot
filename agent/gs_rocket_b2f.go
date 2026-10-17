package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const gsRocketB2FInteractionAttempts = 3

// gsRocketB2FInteract settles the actual retail background/object script
// rather than marking success merely because an interaction was attempted.
// Resume from a checkpoint with an in-flight script before routing anew.
func gsRocketB2FInteract(m *emu.Emu, romData []byte, profile *gsprofile.Profile,
	standX, standY, targetX, targetY uint8, label string, complete func() bool,
) error {
	b2f, err := gsOpeningMapID("TEAM_ROCKET_BASE_B2F")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsRocketB2FInteractionAttempts; attempt++ {
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
				return fmt.Errorf("gen2 Rocket B2F %s: resume script: %w", label, err)
			}
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(b2f, standX, standY),
			targetX, targetY, label); err != nil {
			return fmt.Errorf("gen2 Rocket B2F %s: approach: %w", label, err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 Rocket B2F %s: settle: %w", label, err)
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Rocket B2F %s did not finish its cartridge event", errGSSecondBadgeUnexpectedState, label)
}

func executeGSRocketTransmitterDoor(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b2f:transmitter-door"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketTransmitterDoorOpened,
		gsprofile.ProgressRocketMurkrowPassword, encounter)
	if err != nil || done {
		return err
	}
	// TeamRocketBaseB2F.asm: bg event (14,12); Murkrow password required.
	return gsRocketB2FInteract(m, romData, profile, 14, 13, 14, 12, encounter,
		func() bool {
			return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketTransmitterDoorOpened)
		})
}

// The executive is a coordinate-triggered scripted trainer battle. Observe the
// post-battle flag, not the hide-object flag or the scene's arrival coordinate.
func executeGSRocketExecutiveF(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b2f:executive-f"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketExecutiveFBeaten,
		gsprofile.ProgressRocketTransmitterDoorOpened, encounter)
	if err != nil || done {
		return err
	}
	b2f, err := gsOpeningMapID("TEAM_ROCKET_BASE_B2F")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsRocketB2FInteractionAttempts; attempt++ {
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
				return err
			}
		}
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketExecutiveFBeaten) &&
			profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if err := gsSecondBadgeGoTo(m, romData, profile,
			skill.ExactNativeDestination(b2f, 14, 11)); err != nil {
			return fmt.Errorf("gen2 Rocket B2F executive: approach scene: %w", err)
		}
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
				return err
			}
		}
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketExecutiveFBeaten) &&
			profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Rocket B2F female executive post-win flag absent", errGSSecondBadgeUnexpectedState)
}

// An Electrode battle is wild, but scripted and mandatory. Losses/interruptions
// remain errors; event bits prove which individual Electrode still needs work.
func executeGSRocketElectrodes(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b2f:electrodes"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketHideoutCleared,
		gsprofile.ProgressRocketExecutiveFBeaten, encounter)
	if err != nil || done {
		return err
	}
	for _, step := range []struct {
		label           string
		standY, targetY uint8
		index           int
	}{
		{"electrode-1", 5, 5, 0},
		{"electrode-2", 7, 7, 1},
		{"electrode-3", 9, 9, 2},
	} {
		story := profile.DecodeRocketB2FStory(m)
		if story.Cleared && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if story.Electrodes[step.index] {
			continue
		}
		if err := gsRocketB2FInteract(m, romData, profile, 8, step.standY, 7, step.targetY,
			encounter+":"+step.label,
			func() bool { return profile.DecodeRocketB2FStory(m).Electrodes[step.index] }); err != nil {
			return err
		}
	}
	// The last Electrode starts a separate, potentially interrupted Lance
	// dialogue/HM06 item gift. Never claim victory on Electrode flags alone.
	if !profile.DecodeOverworld(m).Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, encounter+":lance"); err != nil {
			return err
		}
	}
	if !profile.DecodeRocketB2FStory(m).Cleared ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Lance's Rocket Hideout clear and HM06 reward script did not finish", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
