package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	// pokegold/maps/LakeOfRage.asm: Gyarados (18,22), Lance (21,28).
	// The shoreline-facing Gyarados approach requires Surf; route semantics
	// must not pretend the lake is a land corridor.
	gsRedGyaradosX      uint8 = 18
	gsRedGyaradosY      uint8 = 22
	gsRedGyaradosStandX uint8 = 18
	gsRedGyaradosStandY uint8 = 23
	gsLakeLanceX        uint8 = 21
	gsLakeLanceY        uint8 = 28
	gsLakeLanceStandX   uint8 = 21
	gsLakeLanceStandY   uint8 = 29

	// Gen-II A input directly after an overworld turn can be ignored even
	// though the player is technically controllable. Retry only the same
	// owned interaction and require the cartridge's event postcondition.
	gsLakeScriptSettleFrames  = 30
	gsLakeScriptPressAttempts = 3
)

// executeGSRedGyarados is the first executable post-Jasmine story boundary.
// The forced-shiny level-30 encounter is settled by the shared combat driver.
// The object-disappearance event (not simply winning a battle) is the durable
// goal: the ROM sets it before presenting the Red Scale reward and Lance.
func executeGSRedGyarados(m *emu.Emu, romData []byte) error {
	const encounter = "lake-of-rage:red-gyarados"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRedGyaradosDefeated, gsprofile.ProgressMineralBadgeEarned, encounter)
	if err != nil || done {
		return err
	}
	if err := gsCoastalHeal(m, romData, profile, "MAHOGANY_POKECENTER_1F"); err != nil {
		return fmt.Errorf("gen2 Red Gyarados: heal at Mahogany: %w", err)
	}
	if _, err := skill.EnsureFieldMove(m, skill.FieldSurf); err != nil {
		return fmt.Errorf("gen2 Red Gyarados: prepare Surf: %w", err)
	}
	lake, err := gsOpeningMapID("LAKE_OF_RAGE")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsLakeScriptPressAttempts; attempt++ {
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(lake, gsRedGyaradosStandX, gsRedGyaradosStandY),
			gsRedGyaradosX, gsRedGyaradosY, encounter); err != nil {
			return fmt.Errorf("gen2 Red Gyarados: reach lake encounter: %w", err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Red Gyarados: encounter and Red Scale script: %w", err)
		}
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRedGyaradosDefeated) &&
			profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Red Gyarados object-disappearance event was never set", errGSSecondBadgeUnexpectedState)
}

// executeGSLakeOfRageLance follows the finished Red Gyarados encounter.
// The yes/no prompt defaults to YES in retail Gold/Silver. The owned
// interruption driver advances it, but only EVENT_DECIDED_TO_HELP_LANCE
// proves the player actually accepted the Rocket investigation.
func executeGSLakeOfRageLance(m *emu.Emu, romData []byte) error {
	const encounter = "lake-of-rage:lance"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressLanceHelpAccepted, gsprofile.ProgressRedGyaradosDefeated, encounter)
	if err != nil || done {
		return err
	}
	lake, err := gsOpeningMapID("LAKE_OF_RAGE")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsLakeScriptPressAttempts; attempt++ {
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(lake, gsLakeLanceStandX, gsLakeLanceStandY),
			gsLakeLanceX, gsLakeLanceY, encounter); err != nil {
			return fmt.Errorf("gen2 Lance: approach Lake of Rage: %w", err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Lance: accept investigation dialogue: %w", err)
		}
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressLanceHelpAccepted) &&
			profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Lance investigation event was not accepted", errGSSecondBadgeUnexpectedState)
}
