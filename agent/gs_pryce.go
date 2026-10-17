package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// The Mahogany gym is blocked by an NPC until the Rocket Hideout is cleared.
// Never start a leader battle by assuming entering Mahogany is sufficient.
const (
	gsPryceX      uint8 = 5
	gsPryceY      uint8 = 3
	gsPryceStandX uint8 = 5
	gsPryceStandY uint8 = 4
)

// executeGSPryce settles the retail badge script and verifies the cartridge's
// actual ENGINE_GLACIERBADGE bit. EVENT_BEAT_PRYCE alone cannot complete the
// goal: the ROM sets it before the badge reward script finishes.
//
// The Rocket-cleared prerequisite is intentionally required as a separate
// durable objective fact; callers must never bypass the Hideout storyline.
func executeGSPryce(m *emu.Emu, romData []byte) error {
	const encounter = "gym:pryce"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressGlacierBadgeEarned, gsprofile.ProgressRocketHideoutCleared, encounter)
	if err != nil || done {
		return err
	}
	if err := gsCoastalHeal(m, romData, profile, "MAHOGANY_POKECENTER_1F"); err != nil {
		return fmt.Errorf("gen2 Pryce: heal at Mahogany: %w", err)
	}
	gym, err := gsOpeningMapID("MAHOGANY_GYM")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsLakeScriptPressAttempts; attempt++ {
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(gym, gsPryceStandX, gsPryceStandY),
			gsPryceX, gsPryceY, encounter); err != nil {
			return fmt.Errorf("gen2 Pryce: navigate ice gym and face leader: %w", err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Pryce: leader battle and Glacier script: %w", err)
		}
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressGlacierBadgeEarned) &&
			profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Pryce's battle returned without ENGINE_GLACIERBADGE", errGSSecondBadgeUnexpectedState)
}
