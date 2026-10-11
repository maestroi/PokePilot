package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
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
	gsLeadGrindMaxSteps   = 8000
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
// survive Scyther.
func gsEnsureLeadReadyForBugsy(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	return gsEnsureLeadLevel(m, romData, profile, "Bugsy", gsBugsyReadyLeadLevel, "SLOWPOKE_WELL_B1F", 5, 3)
}

// gsEnsureLeadLevel steps around one encounter tile until the lead reaches
// target. Grass/cave tiles roll wild encounters per step, so a short GoTo loop
// under-samples; owned interruption handling covers each battle. The party is
// recovered before and after, and whenever the lead drops below 40% HP.
func gsEnsureLeadLevel(m *emu.Emu, romData []byte, profile *gsprofile.Profile, label string, target int, mapName string, x, y uint8) error {
	level, err := gsLeadLevel(profile, m, romData)
	if err != nil {
		return err
	}
	if level >= target {
		return nil
	}
	grind, err := gsOpeningMapID(mapName)
	if err != nil {
		return err
	}
	tile := skill.ExactNativeDestination(grind, x, y)
	if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
		return fmt.Errorf("recover before %s grind: %w", label, err)
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, tile); err != nil {
		return fmt.Errorf("reach %s grind tile: %w", label, err)
	}

	dirs := []emu.Button{emu.Down, emu.Up, emu.Right, emu.Left, emu.Down, emu.Up}
	for steps := 0; level < target; steps++ {
		if steps >= gsLeadGrindMaxSteps {
			return fmt.Errorf("%w: %s grind reached %d steps with lead still level %d (want %d)",
				errGSSecondBadgeStalled, label, steps, level, target)
		}
		world := profile.DecodeOverworld(m)
		if world.NativeMapID != grind {
			if err := gsSecondBadgeGoTo(m, romData, profile, tile); err != nil {
				return fmt.Errorf("return to %s grind map: %w", label, err)
			}
		}
		if frac, err := gsLeadHPFraction(profile, m, romData); err == nil && frac < 0.4 {
			if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
				return fmt.Errorf("recover during %s grind: %w", label, err)
			}
			if err := gsSecondBadgeGoTo(m, romData, profile, tile); err != nil {
				return fmt.Errorf("return to %s grind after heal: %w", label, err)
			}
			continue
		}
		m.Tap(dirs[steps%len(dirs)], 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, label+"-grind"); err != nil {
			resume, afterBlackout := gsResumeGrindAfterInterruption(err)
			if !resume {
				return fmt.Errorf("%s grind step: %w", label, err)
			}
			if afterBlackout {
				// Wild blackouts are ordinary Gen2 grind outcomes (Confuse Ray
				// self-hits on a lone lead), not required-win combat_defeat.
				// Heal at the Center and resume so combat-recovery training can
				// still reach its level postcondition
				// (run-36loaokprjdtw2j8om666lswgf / triage:734725b12d2a0028).
				if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
					return fmt.Errorf("recover after %s grind blackout: %w", label, err)
				}
				if err := gsSecondBadgeGoTo(m, romData, profile, tile); err != nil {
					return fmt.Errorf("return to %s grind after blackout: %w", label, err)
				}
			}
		}
		level, err = gsLeadLevel(profile, m, romData)
		if err != nil {
			return err
		}
	}
	return gsEnsurePartyRecovered(m, romData, profile)
}

// gsResumeGrindAfterInterruption classifies a grind-step interruption.
// Wild blackouts resume after a Center heal (afterBlackout); draws/flees
// resume in place; trainer losses and non-battle defects fail the session.
func gsResumeGrindAfterInterruption(err error) (resume, afterBlackout bool) {
	var required *skill.RequiredBattleError
	if !errors.As(err, &required) {
		return false, false
	}
	switch required.Outcome.Result {
	case game.BattleLost:
		if required.Outcome.Trainer {
			return false, false
		}
		return true, true
	default:
		// Roar/Whirlwind/flee ends without a win; keep sampling encounters.
		return true, false
	}
}
