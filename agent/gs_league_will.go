package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsWillX        uint8 = 5
	gsWillY        uint8 = 7
	gsWillStandX   uint8 = 5
	gsWillStandY   uint8 = 8
	gsWillAttempts       = 4
)

// executeGSLeagueWill begins the retail Elite Four challenge without faking
// battle completion or stepping through the scripted door behind Will.
// The Indigo Center's MAPCALLBACK_NEWMAP resets the League battle events;
// a checkpoint already inside Will's locked room must therefore resume there
// instead of attempting to leave for another heal and restarting the run.
func executeGSLeagueWill(m *emu.Emu, romData []byte) error {
	const label = "league:will"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressLeagueWillDefeated,
		gsprofile.ProgressIndigoPlateauReached, label)
	if err != nil || done {
		return err
	}
	willRoom, err := gsOpeningMapID("WILLS_ROOM")
	if err != nil {
		return err
	}
	if profile.DecodeOverworld(m).NativeMapID != willRoom {
		if err := gsCoastalHeal(m, romData, profile, "INDIGO_PLATEAU_POKECENTER_1F"); err != nil {
			return fmt.Errorf("gen2 Will: recover at Indigo Plateau before entering the League: %w", err)
		}
	}
	reached := func() bool {
		return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressLeagueWillDefeated)
	}
	for attempt := 0; attempt < gsWillAttempts; attempt++ {
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
				return fmt.Errorf("gen2 Will: settle entrance or battle: %w", err)
			}
		}
		if reached() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(willRoom, gsWillStandX, gsWillStandY),
			gsWillX, gsWillY, label); err != nil {
			return fmt.Errorf("gen2 Will: reach the first Elite Four trainer: %w", err)
		}
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 Will: trainer battle or post-battle script: %w", err)
		}
		if reached() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Will's retail EVENT_BEAT_ELITE_4_WILL was never set", errGSSecondBadgeUnexpectedState)
}
