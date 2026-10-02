package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsWhitneyX      uint8 = 8
	gsWhitneyY      uint8 = 3
	gsWhitneyStandX uint8 = 8
	gsWhitneyStandY uint8 = 4
	gsWhitneyCryX   uint8 = 8
	gsWhitneyCryY   uint8 = 5
)

// executeGSWhitney advances the supported Gen-II frontier through Goldenrod
// Gym. Whitney's retail script deliberately does not award Plain Badge at the
// end of the battle: she cries, Bridget walks over once the player steps onto
// (8,5), and only a later conversation awards the badge. Keep those boundaries
// explicit so a checkpoint can resume after any one of them without treating
// EVENT_BEAT_WHITNEY as overall objective completion.
func executeGSWhitney(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Whitney: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressPlainBadgeEarned) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "gym:whitney")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressTM02HeadbuttAcquired) {
		return fmt.Errorf("%w: Whitney requires the north-Ilex Headbutt boundary", errGSSecondBadgeUnexpectedState)
	}

	// Stage through Goldenrod's Center before entering the gym. Routing from
	// north Ilex naturally owns Route 34 trainer/wild interruptions on the way.
	center, err := gsOpeningMapID("GOLDENROD_POKECENTER_1F")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(center, gsPokecenterStandX, gsPokecenterStandY)); err != nil {
		return fmt.Errorf("gen2 Whitney: reach Goldenrod Center: %w", err)
	}
	if !profile.DecodeCenter(m).Recovered {
		if err := skill.Face(m, gsPokecenterCounterX, gsPokecenterCounterY); err != nil {
			return fmt.Errorf("gen2 Whitney: face Goldenrod nurse: %w", err)
		}
		if err := skill.Heal(m); err != nil {
			return fmt.Errorf("gen2 Whitney: heal before gym: %w", err)
		}
	}

	gym, err := gsOpeningMapID("GOLDENROD_GYM")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(gym, gsWhitneyStandX, gsWhitneyStandY)); err != nil {
		return fmt.Errorf("gen2 Whitney: reach leader: %w", err)
	}

	// Gym trainer sightlines may have cost HP on the way in. Re-use the
	// cartridge-native Center transaction and then return to Whitney.
	if !profile.DecodeCenter(m).Recovered {
		if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
			return fmt.Errorf("gen2 Whitney: recover party before leader: %w", err)
		}
		if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(gym, gsWhitneyStandX, gsWhitneyStandY)); err != nil {
			return fmt.Errorf("gen2 Whitney: return to leader: %w", err)
		}
	}

	// This interaction is also resume-safe after a Whitney win: while
	// EVENT_MADE_WHITNEY_CRY remains set it only drains the "meanie" text.
	if err := skill.Face(m, gsWhitneyX, gsWhitneyY); err != nil {
		return fmt.Errorf("gen2 Whitney: face leader: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "gym:whitney"); err != nil {
		return fmt.Errorf("gen2 Whitney: battle/script: %w", err)
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressPlainBadgeEarned) {
		return nil
	}

	// After a win, GoldenrodGym's scene 1 installs WhitneyCriesScript at (8,5).
	// Walking onto it clears EVENT_MADE_WHITNEY_CRY and restores scene NOOP.
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(gym, gsWhitneyCryX, gsWhitneyCryY)); err != nil {
		return fmt.Errorf("gen2 Whitney: settle crying scene: %w", err)
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(gym, gsWhitneyStandX, gsWhitneyStandY)); err != nil {
		return fmt.Errorf("gen2 Whitney: return after crying scene: %w", err)
	}
	if err := skill.Face(m, gsWhitneyX, gsWhitneyY); err != nil {
		return fmt.Errorf("gen2 Whitney: face leader for badge: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "gym:whitney-badge"); err != nil {
		return fmt.Errorf("gen2 Whitney: badge script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressPlainBadgeEarned) {
		return fmt.Errorf("%w: Whitney returned control without Plain Badge", errGSSecondBadgeUnexpectedState)
	}
	if !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Whitney completed without stable overworld control", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
