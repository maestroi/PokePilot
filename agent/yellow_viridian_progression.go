package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/skill"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowsym "github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowViridianCityMap                  uint8 = 0x01
	yellowViridianCatchTrainingApproachX         = 19
	yellowViridianCatchTrainingApproachY         = 10
	yellowViridianCatchTrainingTriggerX          = 19
	yellowViridianCatchTrainingTriggerY          = 9
	yellowViridianCatchTrainingFrameBudget       = 30000
)

// completeYellowViridianCatchTraining owns the one early-Kanto story beat that
// Yellow inserts between the shared Pokédex transaction and the shared trip to
// Brock. At (19,9) the waiting Old Man starts a scripted BATTLE_TYPE_OLD_MAN
// capture demonstration. That battle has no player-owned FIGHT/ITEM decision,
// so this controller deliberately lets the ROM's simulated input drive it and
// only pages the surrounding known story text.
func completeYellowViridianCatchTraining(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("yellow Viridian catch training: nil emulator")
	}

	observe := func() (Observation, error) {
		raw, err := yellowprofile.New().DecodeObservation(m, romData)
		if err != nil {
			return Observation{}, err
		}
		return observationFromProfile(raw, romData)
	}

	obs, err := observe()
	if err != nil {
		return fmt.Errorf("yellow Viridian catch training: observe: %w", err)
	}
	if obs.Story.Has(yellowprofile.ProgressYellowViridianCatchTraining) {
		return nil
	}
	if !obs.Story.Has(gen1.ProgressPokedexAcquired) {
		return fmt.Errorf("yellow Viridian catch training: Pokedex is not acquired")
	}

	approach := skill.ExactDestination(
		yellowViridianCityMap,
		yellowViridianCatchTrainingApproachX,
		yellowViridianCatchTrainingApproachY,
	)
	if _, err := skill.TravelFlee(m, romData, approach, skill.StatAwareMove(romData), 40); err != nil {
		return fmt.Errorf("yellow Viridian catch training: reach Old Man approach: %w", err)
	}

	trigger := skill.ExactDestination(
		yellowViridianCityMap,
		yellowViridianCatchTrainingTriggerX,
		yellowViridianCatchTrainingTriggerY,
	)
	if err := skill.GoTo(m, romData, trigger); err != nil &&
		!errors.Is(err, skill.ErrDialogueInterrupted) &&
		!errors.Is(err, skill.ErrBattleInterrupted) &&
		!errors.Is(err, skill.ErrBattle) {
		return fmt.Errorf("yellow Viridian catch training: enter trigger: %w", err)
	}

	start := m.FrameCount()
	for int(m.FrameCount()-start) <= yellowViridianCatchTrainingFrameBudget {
		obs, err := observe()
		if err != nil {
			return fmt.Errorf("yellow Viridian catch training: observe script: %w", err)
		}
		if obs.Story.Has(yellowprofile.ProgressYellowViridianCatchTraining) &&
			obs.Controllable && !obs.InBattle && m.Peek8(yellowsym.FontLoaded) == 0 {
			return nil
		}

		if obs.InBattle {
			// The cartridge owns every menu input during BATTLE_TYPE_OLD_MAN.
			m.StepFrame()
			continue
		}
		if m.Peek8(yellowsym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
			continue
		}
		m.StepFrame()
	}

	return fmt.Errorf("yellow Viridian catch training: exceeded %d frames",
		yellowViridianCatchTrainingFrameBudget)
}

func executeYellowEarlyProgression(m *emu.Emu, romData []byte, o Objective) error {
	switch o.Progress {
	case gen1.ProgressPokedexAcquired:
		return gen1AcquirePokedex(m, romData, skill.StatAwareMove(romData))
	case gen1.ProgressBoulderBadge:
		if err := completeYellowViridianCatchTraining(m, romData); err != nil {
			return err
		}
		return gen1DefeatBrock(m, romData, skill.StatAwareMove(romData))
	default:
		return fmt.Errorf("yellow early progression: unsupported goal %q", o.Progress)
	}
}
