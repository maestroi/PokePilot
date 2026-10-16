package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// executeGSMahoganyStairs owns the very first Rocket Hideout transaction:
// after Lance agrees to investigate, entering Mahogany Mart triggers his
// Dragonite scene. It transforms the shop block (6,2) into real stairs,
// and the cartridge sets EVENT_UNCOVERED_STAIRCASE_IN_MAHOGANY_MART.
//
// The scene is not complete just because the player reached the store.
// Resume from save/replay by recognizing the permanent event bit, and keep
// subsequent Rocket Base password/door/camera/battle scripts outside this
// limited objective until they have their own controllers.
func executeGSMahoganyStairs(m *emu.Emu, romData []byte) error {
	const encounter = "mahogany:rocket-base-stairs"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressMahoganyStairsOpened, gsprofile.ProgressLanceHelpAccepted, encounter)
	if err != nil || done {
		return err
	}
	mart, err := gsOpeningMapID("MAHOGANY_MART_1F")
	if err != nil {
		return err
	}
	// The Lance/Dragonite cutscene is automatically queued on map entry.
	// Our owned interruption driver settles only this Mart scene; no generic
	// script recovery is allowed to wander downstairs before the flag flips.
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(mart, 3, 6)); err != nil {
		return fmt.Errorf("gen2 Mahogany Mart: reach Lance reveal: %w", err)
	}
	if !profile.DecodeOverworld(m).Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Mahogany Mart: settle Lance reveal: %w", err)
		}
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressMahoganyStairsOpened) ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Mahogany Mart scene did not uncover Rocket staircase", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
