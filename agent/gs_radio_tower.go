package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsRadioTowerFakeDirectorSceneX   uint8 = 0
	gsRadioTowerFakeDirectorSceneY   uint8 = 3
	gsRadioTowerFakeDirectorAttempts       = 3
)

// executeGSRadioTowerFakeDirector handles only the first retail Radio Tower
// takeover objective. Stepping onto (0,3) in 5F triggers the impostor's
// dialogue and EXECUTIVEM_3 battle. The post-battle script awards Basement
// Key BEFORE setting EVENT_BEAT_ROCKET_EXECUTIVEM_3, which is the durable
// proof for this transaction. Reaching 5F or winning a generic trainer battle
// must never substitute for that event.
//
// Available for explicitly opted-in seven/eight-badge campaigns; still
// subject to retail ROM qualification and genuine postcondition checks.
func executeGSRadioTowerFakeDirector(m *emu.Emu, romData []byte) error {
	const encounter = "radio-tower:fake-director"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRadioTowerFakeDirectorBeaten,
		gsprofile.ProgressGlacierBadgeEarned, encounter)
	if err != nil || done {
		return err
	}
	if err := gsCoastalHeal(m, romData, profile, "GOLDENROD_POKECENTER_1F"); err != nil {
		return fmt.Errorf("gen2 Radio Tower: heal in Goldenrod: %w", err)
	}
	fifth, err := gsOpeningMapID("RADIO_TOWER_5F")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsRadioTowerFakeDirectorAttempts; attempt++ {
		if err := gsSecondBadgeGoTo(m, romData, profile,
			skill.ExactNativeDestination(fifth,
				gsRadioTowerFakeDirectorSceneX, gsRadioTowerFakeDirectorSceneY)); err != nil {
			return fmt.Errorf("gen2 Radio Tower: reach 5F fake-director scene: %w", err)
		}
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
				return fmt.Errorf("gen2 Radio Tower: settle fake-director battle and key script: %w", err)
			}
		}
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRadioTowerFakeDirectorBeaten) &&
			profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Radio Tower fake director did not award Basement Key and set EXECUTIVEM_3 event",
		errGSSecondBadgeUnexpectedState)
}
