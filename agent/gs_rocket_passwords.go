package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const gsRocketInteractionAttempts = 3

// gsRocketB3FTalk settles the actual retail trainer or character script at a
// pinned B3F object location. A battle can happen before its password gift;
// the shared interruption driver must settle both before verification.
func gsRocketB3FTalk(m *emu.Emu, romData []byte, profile *gsprofile.Profile,
	standX, standY, targetX, targetY uint8, encounter string,
	completed func() bool,
) error {
	mapID, err := gsOpeningMapID("TEAM_ROCKET_BASE_B3F")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsRocketInteractionAttempts; attempt++ {
		if completed() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(mapID, standX, standY),
			targetX, targetY, encounter); err != nil {
			return fmt.Errorf("gen2 Rocket B3F: approach %s: %w", encounter, err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Rocket B3F: settle %s: %w", encounter, err)
		}
		if completed() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Rocket B3F %s script did not set expected ROM event", errGSSecondBadgeUnexpectedState, encounter)
}

func executeGSRocketPasswords(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b3f:passwords"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketPasswordsLearned, gsprofile.ProgressRocketLanceHealing, encounter)
	if err != nil || done {
		return err
	}
	// From pokegold/maps/TeamRocketBaseB3F.asm: the RaticateTail
	// male grunt at (5,15), then the SlowpokeTail female at (21,7).
	// Winning alone does not prove learning either password: their post-win
	// conversations set two separate permanent events.
	for _, script := range []struct {
		label                    string
		standX, standY, x, y     uint8
		fact                     game.ProgressID
	}{
		{"raticate-tail", 5, 14, 5, 15, gsprofile.ProgressRocketRaticatePassword},
		{"slowpoke-tail", 21, 8, 21, 7, gsprofile.ProgressRocketSlowpokePassword},
	} {
		if gsFirstBadgeProgressComplete(profile, m, script.fact) {
			continue
		}
		if err := gsRocketB3FTalk(m, romData, profile,
			script.standX, script.standY, script.x, script.y,
			encounter+":"+script.label,
			func() bool { return gsFirstBadgeProgressComplete(profile, m, script.fact) }); err != nil {
			return err
		}
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketPasswordsLearned) {
		return fmt.Errorf("%w: both retail Rocket passwords are required", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

func executeGSRocketOfficeDoor(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b3f:office-door"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketOfficeOpened, gsprofile.ProgressRocketPasswordsLearned, encounter)
	if err != nil || done {
		return err
	}
	// Door event at (10,9): both passwords unlock the B3F floor tile.
	return gsRocketB3FTalk(m, romData, profile, 10, 10, 10, 9, encounter,
		func() bool { return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketOfficeOpened) })
}

func executeGSRocketExecutiveM4(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b3f:executive"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketExecutiveBeaten, gsprofile.ProgressRocketOfficeOpened, encounter)
	if err != nil || done {
		return err
	}
	b3f, err := gsOpeningMapID("TEAM_ROCKET_BASE_B3F")
	if err != nil {
		return err
	}
	// Stepping on (10,8) triggers RocketBaseBossLeft. The scene owns the
	// encounter and defeat flag (not a synthetic trainer victory).
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(b3f, 10, 8)); err != nil {
		return fmt.Errorf("gen2 Rocket B3F: reach Giovanni office executive: %w", err)
	}
	if !profile.DecodeOverworld(m).Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Rocket B3F: settle executive battle: %w", err)
		}
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketExecutiveBeaten) ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Giovanni office executive win event missing", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

func executeGSRocketMurkrow(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b3f:murkrow"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketMurkrowPassword, gsprofile.ProgressRocketExecutiveBeaten, encounter)
	if err != nil || done {
		return err
	}
	return gsRocketB3FTalk(m, romData, profile, 7, 3, 7, 2, encounter,
		func() bool { return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketMurkrowPassword) })
}
