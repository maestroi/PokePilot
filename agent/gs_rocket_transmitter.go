package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// Pinned to pret/pokegold/maps/TeamRocketBaseB2F.asm. The transmitter
// door is a background event; the executive is a coordinate-triggered scene.
const (
	gsRocketTransmitterDoorX  uint8 = 14
	gsRocketTransmitterDoorY  uint8 = 12
	gsRocketExecutiveTriggerX uint8 = 14
	gsRocketExecutiveTriggerY uint8 = 11
)

func gsRocketB2FInteract(m *emu.Emu, romData []byte, profile *gsprofile.Profile,
	standX, standY, targetX, targetY uint8, encounter string, completed func() bool,
) error {
	b2f, err := gsOpeningMapID("TEAM_ROCKET_BASE_B2F")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsRocketInteractionAttempts; attempt++ {
		if completed() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(b2f, standX, standY),
			targetX, targetY, encounter); err != nil {
			return fmt.Errorf("gen2 Rocket B2F: stage %s: %w", encounter, err)
		}
		// Gen-II may ignore A immediately after a direction change.
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Rocket B2F: settle %s: %w", encounter, err)
		}
		if completed() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Rocket B2F %s did not complete its cartridge event",
		errGSSecondBadgeUnexpectedState, encounter)
}

// EVENT_OPENED_DOOR_TO_ROCKET_HIDEOUT_TRANSMITTER, not arrival on B2F,
// proves the Murkrow password opened the locked transmitter doorway.
func executeGSRocketTransmitterDoor(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b2f:transmitter-door"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketTransmitterDoor, gsprofile.ProgressRocketMurkrowPassword, encounter)
	if err != nil || done {
		return err
	}
	return gsRocketB2FInteract(m, romData, profile,
		gsRocketTransmitterDoorX, gsRocketTransmitterDoorY+1,
		gsRocketTransmitterDoorX, gsRocketTransmitterDoorY, encounter,
		func() bool { return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketTransmitterDoor) })
}

// Stepping on (14,11) starts the executive battle with Lance fighting the
// other Rocket. Winning is necessary but the durable post-battle event is
// the only evidence that the scripted aftermath actually completed.
func executeGSRocketExecutiveF(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b2f:executive-f"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketExecutiveF, gsprofile.ProgressRocketTransmitterDoor, encounter)
	if err != nil || done {
		return err
	}
	b2f, err := gsOpeningMapID("TEAM_ROCKET_BASE_B2F")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(b2f, gsRocketExecutiveTriggerX, gsRocketExecutiveTriggerY)); err != nil {
		return fmt.Errorf("gen2 Rocket B2F: reach executive event: %w", err)
	}
	m.StepFrames(gsLakeScriptSettleFrames)
	if !profile.DecodeOverworld(m).Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Rocket B2F: settle executive and Lance scene: %w", err)
		}
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketExecutiveF) ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Rocket B2F executive victory event absent", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

// Lance handles the right-side Electrode, while the player clears the three
// left-side objects (7,5), (7,7), (7,9). The final encounter starts Lance's
// HM06 handoff. A loss never sets an object-disappearance event and instead
// propagates the existing required-battle outcome for recovery/re-planning.
func executeGSRocketElectrodes(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b2f:electrodes"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketHideoutCleared, gsprofile.ProgressRocketExecutiveF, encounter)
	if err != nil || done {
		return err
	}
	for _, electrode := range []struct {
		fact game.ProgressID
		y    uint8
	}{
		{gsprofile.ProgressRocketElectrode1, 5},
		{gsprofile.ProgressRocketElectrode2, 7},
		{gsprofile.ProgressRocketElectrode3, 9},
	} {
		if gsFirstBadgeProgressComplete(profile, m, electrode.fact) {
			continue
		}
		label := fmt.Sprintf("%s:%d", encounter, electrode.y)
		if err := gsRocketB2FInteract(m, romData, profile,
			8, electrode.y, 7, electrode.y, label,
			func() bool { return gsFirstBadgeProgressComplete(profile, m, electrode.fact) }); err != nil {
			return err
		}
	}
	// HM06 and the radio signal are settled by the retail post-third-Electrode
	// script; do not report completion from a trainer win or three flags alone.
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketHideoutCleared) ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: all three Electrode and Lance's Whirlpool handoff required",
			errGSSecondBadgeUnexpectedState)
	}
	return nil
}
