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
	name            string
	mapName         string
	standX, standY  uint8
	targetX, targetY uint8
	complete        func(gsprofile.RocketBaseStory) bool
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

// executeGSRocketHideout resumes each durable password/boss/Electrode stage.
// It refuses to report victory until Lance's post-Electrode Whirlpool reward
// script has set EVENT_CLEARED_ROCKET_HIDEOUT (event 34).
func executeGSRocketHideout(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-base:clear"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketHideoutCleared, gsprofile.ProgressRocketLanceHealing, encounter)
	if err != nil || done {
		return err
	}

	// Defeating the two password grunts is not sufficient. Talking to them
	// after their trainer battle is what sets the actual password flags.
	for _, step := range []gsRocketInteraction{
		{
			name: "slowpoketail", mapName: "TEAM_ROCKET_BASE_B3F",
			standX: 21, standY: 8, targetX: 21, targetY: 7,
			complete: func(s gsprofile.RocketBaseStory) bool { return s.SlowpokeTail },
		},
		{
			name: "raticate-tail", mapName: "TEAM_ROCKET_BASE_B3F",
			standX: 5, standY: 16, targetX: 5, targetY: 15,
			complete: func(s gsprofile.RocketBaseStory) bool { return s.RaticateTail },
		},
		{
			name: "giovanni-office-door", mapName: "TEAM_ROCKET_BASE_B3F",
			standX: 10, standY: 10, targetX: 10, targetY: 9,
			complete: func(s gsprofile.RocketBaseStory) bool { return s.GiovanniDoor },
		},
	} {
		if err := gsRocketSpeak(m, romData, profile, step); err != nil {
			return err
		}
	}

	// Giovanni's office executive, unlike a normal trainer, is entered via
	// a B3F coord-event and must complete its post-battle escape script.
	if err := gsRocketEnterScene(m, romData, profile, "TEAM_ROCKET_BASE_B3F", 10, 8,
		"executive-m", func(s gsprofile.RocketBaseStory) bool { return s.ExecutiveM }); err != nil {
		return err
	}
	if err := gsRocketSpeak(m, romData, profile, gsRocketInteraction{
		name: "murkrow-password", mapName: "TEAM_ROCKET_BASE_B3F",
		standX: 7, standY: 3, targetX: 7, targetY: 2,
		complete: func(s gsprofile.RocketBaseStory) bool { return s.HailGiovanni },
	}); err != nil {
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

	// Each Electrode is a separate scripted forced encounter, not a normal
	// trainer sightline. A blackout must never be treated as an Electrode clear.
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
		// The last encounter triggers Lance's reward and cleanup scene
		// automatically. It is possible for all Electrodes to be gone but
		// the reward dialogue to be unsettled at a save boundary.
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
		!gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketHideoutCleared) {
		return fmt.Errorf("%w: Lance did not finish the Rocket Base clearance/Whirlpool script", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
