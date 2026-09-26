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
	yellowMtMoonB2FMap       uint8 = 0x3d
	yellowMtMoonJessieJamesX uint8 = 3
	yellowMtMoonJessieJamesY uint8 = 5
	yellowMtMoonExitBudget         = 30000
)

func yellowStoryHas(m *emu.Emu, romData []byte, id ProgressID) (bool, error) {
	obs, err := yellowprofile.New().DecodeObservation(m, romData)
	if err != nil {
		return false, err
	}
	return obs.Story.Has(id), nil
}

// executeYellowMtMoonExit owns only Yellow's extra Jessie/James interruption.
// Fossil acquisition itself is the shared Gen-I transaction; Yellow inserts
// this mandatory battle immediately afterwards and does not consider Mt. Moon
// traversable until its own semantic exit fact is durable.
func executeYellowMtMoonExit(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("yellow Mt. Moon exit: nil emulator")
	}
	done, err := yellowStoryHas(m, romData, yellowprofile.ProgressYellowMtMoonExitResolved)
	if err != nil {
		return err
	}
	if done {
		return nil
	}
	fossil, err := yellowStoryHas(m, romData, gen1.ProgressMtMoonFossilAcquired)
	if err != nil {
		return err
	}
	if !fossil {
		return fmt.Errorf("yellow Mt. Moon exit: fossil milestone is incomplete")
	}

	policy := skill.StatAwareMove(romData)
	err = skill.GoTo(m, romData, skill.ExactDestination(yellowMtMoonB2FMap, yellowMtMoonJessieJamesX, yellowMtMoonJessieJamesY))
	if err != nil && !errors.Is(err, skill.ErrBattleInterrupted) && !errors.Is(err, skill.ErrDialogueInterrupted) && !errors.Is(err, skill.ErrBattle) {
		return fmt.Errorf("yellow Mt. Moon exit: reach Jessie/James trigger: %w", err)
	}

	start := m.FrameCount()
	for int(m.FrameCount()-start) <= yellowMtMoonExitBudget {
		obs, observeErr := yellowprofile.New().DecodeObservation(m, romData)
		if observeErr != nil {
			return fmt.Errorf("yellow Mt. Moon exit: observe: %w", observeErr)
		}
		if obs.Story.Has(yellowprofile.ProgressYellowMtMoonExitResolved) &&
			obs.Controllable && !obs.InBattle && m.Peek8(yellowsym.FontLoaded) == 0 {
			return nil
		}
		if obs.InBattle {
			if _, battleErr := skill.Battle(m, policy); battleErr != nil {
				return fmt.Errorf("yellow Mt. Moon exit: Jessie/James battle: %w", battleErr)
			}
			continue
		}
		if m.Peek8(yellowsym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
			continue
		}
		m.StepFrame()
	}
	return fmt.Errorf("yellow Mt. Moon exit: exceeded %d frames before semantic completion", yellowMtMoonExitBudget)
}
