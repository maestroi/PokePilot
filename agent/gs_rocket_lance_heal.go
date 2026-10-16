package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// executeGSRocketBaseLanceHeal is a separate, replayable B1F/B2F boundary.
// The retail B1F camera triggers can cause chained trainer fights; the
// shared interruption driver owns those encounters. On B2F, stepping onto
// (5,14) executes LanceHealsScript1, which restores the party and sets
// EVENT_LANCE_HEALED_YOU_IN_TEAM_ROCKET_BASE (not a transient map scene).
//
// Password quests and the transmitter are deliberately not part of this
// objective. Reaching B2F without settling Lance's script cannot satisfy it.
func executeGSRocketBaseLanceHeal(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-base:lance-heal"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketLanceHealing, gsprofile.ProgressMahoganyStairsOpened, encounter)
	if err != nil || done {
		return err
	}
	b2f, err := gsOpeningMapID("TEAM_ROCKET_BASE_B2F")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(b2f, 5, 14)); err != nil {
		return fmt.Errorf("gen2 Rocket Base: traverse B1F cameras to Lance: %w", err)
	}
	if !profile.DecodeOverworld(m).Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Rocket Base: settle Lance heal: %w", err)
		}
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketLanceHealing) ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: B2F Lance heal event never completed", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
