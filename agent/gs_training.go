package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// gsGrindSpot is an adapter-owned training habitat: an indoor map where every
// floor tile rolls wild encounters, usable once its story gate is open.
type gsGrindSpot struct {
	mapName string
	open    game.ProgressID
}

// Strongest last. Encounter levels from pokegold/data/wild/johto_grass.asm:
// Slowpoke Well B1F 5-8 (2%), Burned Tower 1F 13-16 (4%).
var gsGrindSpots = []gsGrindSpot{
	{mapName: "SLOWPOKE_WELL_B1F", open: gsprofile.ProgressSlowpokeWellCleared},
	{mapName: "BURNED_TOWER_1F", open: gsprofile.ProgressSudowoodoCleared},
}

// gsTrainingSpot returns the strongest grind map the story has opened.
func gsTrainingSpot(story ProgressState) (gsGrindSpot, bool) {
	for i := len(gsGrindSpots) - 1; i >= 0; i-- {
		if story.Has(gsGrindSpots[i].open) {
			return gsGrindSpots[i], true
		}
	}
	return gsGrindSpot{}, false
}

// gsTrainingObjectives offers lead training while a recorded combat loss owes
// readiness. The generic combat-loss gate withholds the lost fight until
// training reaches its readiness target (run-1auv5rq62ou1i16n25pxcc2izv).
func gsTrainingObjectives(obs Observation) []Objective {
	if !obs.CombatLossRecorded || len(obs.Party) == 0 {
		return nil
	}
	spot, ok := gsTrainingSpot(obs.Story)
	if !ok {
		return nil
	}
	target := int(obs.Party[0].Level) + trainStep
	if target > 100 {
		return nil
	}
	return []Objective{{
		Kind:  KindTrain,
		Level: uint8(target),
		Note:  fmt.Sprintf("(combat preparation: grind wild battles in %s, healing at a Pokemon Center as needed)", spot.mapName),
	}}
}

const gsTrainMaxLegs = 6000

// executeGSTrain walks the strongest open grind map until the lead reaches
// level, fighting every encounter through the owned interruption handler and
// healing at a Center when the lead runs low.
func executeGSTrain(m *emu.Emu, romData []byte, level uint8) error {
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	obs, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return err
	}
	spot, ok := gsTrainingSpot(ProgressState(obs.Story))
	if !ok {
		return fmt.Errorf("%w: no Gen-II grind spot is open yet", errGSControllerUnavailable)
	}
	grindMap, err := gsOpeningMapID(spot.mapName)
	if err != nil {
		return err
	}
	reach := func() ([2]uint8, [2]uint8, error) {
		if err := gsSecondBadgeGoTo(m, romData, profile, skill.NativeMapDestination(grindMap)); err != nil {
			return [2]uint8{}, [2]uint8{}, fmt.Errorf("reach %s: %w", spot.mapName, err)
		}
		return skill.NativeGrindPair(m, romData)
	}
	if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
		return fmt.Errorf("recover before training: %w", err)
	}
	a, b, err := reach()
	if err != nil {
		return err
	}
	for legs := 0; ; legs++ {
		lead, err := gsLeadLevel(profile, m, romData)
		if err != nil {
			return err
		}
		if lead >= int(level) {
			break
		}
		if legs >= gsTrainMaxLegs {
			return fmt.Errorf("%w: training reached %d legs with lead still level %d (want %d)",
				errGSSecondBadgeStalled, legs, lead, level)
		}
		if frac, err := gsLeadHPFraction(profile, m, romData); err == nil && frac < 0.4 {
			if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
				return fmt.Errorf("recover during training: %w", err)
			}
			if a, b, err = reach(); err != nil {
				return err
			}
			continue
		}
		if profile.DecodeOverworld(m).NativeMapID != grindMap {
			if a, b, err = reach(); err != nil {
				return err
			}
		}
		next := a
		if legs%2 == 1 {
			next = b
		}
		if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(grindMap, next[0], next[1])); err != nil {
			return fmt.Errorf("training leg: %w", err)
		}
	}
	return gsEnsurePartyRecovered(m, romData, profile)
}
