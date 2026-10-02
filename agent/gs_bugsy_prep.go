package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// Bugsy's Scyther (Lv16, Bug/Flying) outspeeds Bayleef and snowballs Fury
// Cutter. Grass moves are 1/4 in Gen II against that typing, so the lead's
// only real damage is Tackle. Measured from the farm checkpoint: Lv20 full HP
// still loses even with Reflect+Growl. A mid-20s lead with Reflect up can take
// the FC curve and still KO with Tackle.
const (
	gsBugsyReadyLeadLevel = 24
	gsBugsyGrindMaxSteps  = 8000
)

func gsLeadLevel(profile *gsprofile.Profile, m *emu.Emu, romData []byte) (int, error) {
	obs, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return 0, err
	}
	for _, mon := range obs.Party {
		if mon.IsEgg {
			continue
		}
		return int(mon.Level), nil
	}
	return 0, fmt.Errorf("%w: no non-egg party member to grind", errGSSecondBadgeUnexpectedState)
}

func gsLeadHPFraction(profile *gsprofile.Profile, m *emu.Emu, romData []byte) (float64, error) {
	obs, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return 0, err
	}
	for _, mon := range obs.Party {
		if mon.IsEgg || mon.MaxHP == 0 {
			continue
		}
		return float64(mon.HP) / float64(mon.MaxHP), nil
	}
	return 0, fmt.Errorf("%w: no non-egg party member", errGSSecondBadgeUnexpectedState)
}

// gsEnsureLeadReadyForBugsy steps around Slowpoke Well B1F until the lead can
// survive Scyther. Cave tiles roll wild encounters per step, so a short GoTo
// loop under-samples; owned interruption handling covers each battle.
func gsEnsureLeadReadyForBugsy(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	level, err := gsLeadLevel(profile, m, romData)
	if err != nil {
		return err
	}
	if level >= gsBugsyReadyLeadLevel {
		return nil
	}
	well, err := gsOpeningMapID("SLOWPOKE_WELL_B1F")
	if err != nil {
		return err
	}
	if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
		return fmt.Errorf("recover before Bugsy grind: %w", err)
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(well, 5, 3)); err != nil {
		return fmt.Errorf("reach Slowpoke Well grind tile: %w", err)
	}

	dirs := []emu.Button{emu.Down, emu.Up, emu.Right, emu.Left, emu.Down, emu.Up}
	for steps := 0; level < gsBugsyReadyLeadLevel; steps++ {
		if steps >= gsBugsyGrindMaxSteps {
			return fmt.Errorf("%w: Bugsy grind reached %d steps with lead still level %d (want %d)",
				errGSSecondBadgeStalled, steps, level, gsBugsyReadyLeadLevel)
		}
		world := profile.DecodeOverworld(m)
		if world.NativeMapID != well {
			if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(well, 5, 3)); err != nil {
				return fmt.Errorf("return to grind map: %w", err)
			}
		}
		if frac, err := gsLeadHPFraction(profile, m, romData); err == nil && frac < 0.4 {
			if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
				return fmt.Errorf("recover during Bugsy grind: %w", err)
			}
			if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(well, 5, 3)); err != nil {
				return fmt.Errorf("return to grind after heal: %w", err)
			}
			continue
		}
		m.Tap(dirs[steps%len(dirs)], 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, "bugsy-grind"); err != nil {
			return fmt.Errorf("Bugsy grind step: %w", err)
		}
		level, err = gsLeadLevel(profile, m, romData)
		if err != nil {
			return err
		}
	}
	return gsEnsurePartyRecovered(m, romData, profile)
}
