package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsSurgeX                uint8 = 5
	gsSurgeY                uint8 = 2
	gsSurgeStandX           uint8 = 5
	gsSurgeStandY           uint8 = 3
	gsSurgeAttempts               = 3
)

// gsSurgeApproachMap confines the first Kanto badge objective to Vermilion.
// A historical first-voyage event does not prove the player is *currently*
// in Kanto; postgame return-trip routing is not an executable skill yet.
func gsSurgeApproachMap(mapID uint16) bool {
	return gsMapIn(mapID, []string{
		"VERMILION_PORT", "VERMILION_PORT_PASSAGE",
		"VERMILION_CITY", "VERMILION_POKECENTER_1F", "VERMILION_GYM",
	})
}

// executeGSKantoSurge follows the actual Gold/Silver Vermilion Gym script.
// In Gen II the trash-can switch traps are deactivated; the native route to
// (5,3) crosses the trainers' real sightlines and the owned interruption
// driver fights any triggered battles. Only ENGINE_THUNDERBADGE (wKantoBadges
// bit 2), awarded *after* Surge's battle and speech, completes this objective.
// EVENT_BEAT_LTSURGE alone must never count.
func executeGSKantoSurge(m *emu.Emu, romData []byte) error {
	const encounter = "kanto:gym:lt-surge"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoThunderBadgeEarned,
		gsprofile.ProgressKantoFirstLanding, encounter)
	if err != nil || done {
		return err
	}

	world := profile.DecodeOverworld(m)
	if !gsSurgeApproachMap(world.NativeMapID) {
		return fmt.Errorf("%w: Lt. Surge must start from the first Vermilion landing or Gym, not map %#04x",
			errGSSecondBadgeUnexpectedState, world.NativeMapID)
	}
	if !world.Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Lt. Surge: resume local Kanto script: %w", err)
		}
	}

	gym, err := gsOpeningMapID("VERMILION_GYM")
	if err != nil {
		return err
	}
	complete := func() bool {
		return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressKantoThunderBadgeEarned)
	}
	for attempt := 0; attempt < gsSurgeAttempts; attempt++ {
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		// Recover at Vermilion's actual Center before challenging Surge.
		// If prior Gym trainers drained HP/PP, return to heal and re-enter;
		// their retail defeated events remain durable on the return trip.
		if !profile.DecodeCenter(m).Recovered {
			if err := gsCoastalHeal(m, romData, profile, "VERMILION_POKECENTER_1F"); err != nil {
				return fmt.Errorf("gen2 Lt. Surge: heal before Gym: %w", err)
			}
		}
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(gym, gsSurgeStandX, gsSurgeStandY),
			gsSurgeX, gsSurgeY, encounter); err != nil {
			return fmt.Errorf("gen2 Lt. Surge: traverse Vermilion Gym: %w", err)
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Lt. Surge: battle and badge script: %w", err)
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Lt. Surge interaction returned without Kanto Thunder Badge",
		errGSSecondBadgeUnexpectedState)
}
