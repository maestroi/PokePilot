package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// These locations are the object and coord-event positions in the vendored
// Gold/Silver Rocket Base B3F and B2F map scripts. The objective advances only
// on persisted event bits, never on reaching a coordinate or ending a battle.
type gsRocketInteraction struct {
	name             string
	mapName          string
	standX, standY   uint8
	targetX, targetY uint8
	complete         func(gsprofile.RocketBaseStory) bool
}

func gsRocketSpeak(m *emu.Emu, romData []byte, profile *gsprofile.Profile, step gsRocketInteraction) error {
	const maxAttempts = 3
	mapID, err := gsOpeningMapID(step.mapName)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if step.complete(profile.DecodeRocketBaseStory(m)) {
			return nil
		}
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(mapID, step.standX, step.standY),
			step.targetX, step.targetY, step.name); err != nil {
			return fmt.Errorf("Rocket Base %s: approach: %w", step.name, err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, step.name); err != nil {
			return fmt.Errorf("Rocket Base %s: interaction: %w", step.name, err)
		}
	}
	return fmt.Errorf("%w: Rocket Base %s did not set its permanent event", errGSSecondBadgeUnexpectedState, step.name)
}

func gsRocketEnterScene(m *emu.Emu, romData []byte, profile *gsprofile.Profile,
	mapName string, x, y uint8, sceneName string, done func(gsprofile.RocketBaseStory) bool) error {
	mapID, err := gsOpeningMapID(mapName)
	if err != nil {
		return err
	}
	if done(profile.DecodeRocketBaseStory(m)) {
		return nil
	}
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(mapID, x, y)); err != nil {
		return fmt.Errorf("Rocket Base %s: approach scene: %w", sceneName, err)
	}
	if !profile.DecodeOverworld(m).Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, sceneName); err != nil {
			return fmt.Errorf("Rocket Base %s: settle scene: %w", sceneName, err)
		}
	}
	if !done(profile.DecodeRocketBaseStory(m)) {
		return fmt.Errorf("%w: Rocket Base %s scene returned without event", errGSSecondBadgeUnexpectedState, sceneName)
	}
	return nil
}

// executeGSRocketTransmitter continues immediately after the existing B3F
// Murkrow controller. It never refights password grunts or the office boss.
// Each B2F script is checked against a permanent cartridge event and the
// entire objective completes only after Lance's post-Electrode HM06 reward.
func executeGSRocketTransmitter(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-base:transmitter"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketHideoutCleared,
		gsprofile.ProgressRocketMurkrowPassword, encounter)
	if err != nil || done {
		return err
	}
	if err := gsRocketSpeak(m, romData, profile, gsRocketInteraction{
		name: "transmitter-door", mapName: "TEAM_ROCKET_BASE_B2F",
		standX: 14, standY: 13, targetX: 14, targetY: 12,
		complete: func(s gsprofile.RocketBaseStory) bool { return s.TransmitterDoor },
	}); err != nil {
		return err
	}
	if err := gsRocketEnterScene(m, romData, profile, "TEAM_ROCKET_BASE_B2F", 14, 11,
		"executive-f", func(s gsprofile.RocketBaseStory) bool { return s.ExecutiveF }); err != nil {
		return err
	}
	for _, step := range []gsRocketInteraction{
		{
			name: "electrode-1", mapName: "TEAM_ROCKET_BASE_B2F",
			standX: 8, standY: 5, targetX: 7, targetY: 5,
			complete: func(s gsprofile.RocketBaseStory) bool { return s.Electrodes[0] },
		},
		{
			name: "electrode-2", mapName: "TEAM_ROCKET_BASE_B2F",
			standX: 8, standY: 7, targetX: 7, targetY: 7,
			complete: func(s gsprofile.RocketBaseStory) bool { return s.Electrodes[1] },
		},
		{
			name: "electrode-3", mapName: "TEAM_ROCKET_BASE_B2F",
			standX: 8, standY: 9, targetX: 7, targetY: 9,
			complete: func(s gsprofile.RocketBaseStory) bool { return s.Electrodes[2] },
		},
	} {
		if profile.DecodeRocketBaseStory(m).Cleared {
			return nil
		}
		if err := gsRocketSpeak(m, romData, profile, step); err != nil {
			return err
		}
	}
	if !profile.DecodeOverworld(m).Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return err
		}
	}
	if !profile.DecodeRocketBaseStory(m).Cleared ||
		!gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketHideoutCleared) ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Lance's Whirlpool reward did not clear the Rocket signal", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
