package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
)

// scriptedProgressTrigger describes a deterministic map-script handoff whose
// durable contract is a semantic ProgressID. Concrete games own only the
// trigger coordinate and progress fact; navigation, battle resolution,
// dialogue advancement, resumption, and positive postcondition handling are
// shared here.
type scriptedProgressTrigger struct {
	Name        string
	Destination skill.Destination
	Progress    ProgressID
	MaxFrames   int
	IdleFrames  int
	AdvanceA    bool
}

func executeScriptedProgressTrigger(m *emu.Emu, romData []byte, spec scriptedProgressTrigger) error {
	if m == nil {
		return fmt.Errorf("%s: nil emulator", spec.Name)
	}
	if spec.Progress == "" {
		return fmt.Errorf("%s: empty progress postcondition", spec.Name)
	}
	if spec.MaxFrames <= 0 {
		spec.MaxFrames = 30000
	}
	if spec.IdleFrames <= 0 {
		spec.IdleFrames = 90
	}

	obs, err := ObserveChecked(m, romData)
	if err != nil {
		return fmt.Errorf("%s: observe initial state: %w", spec.Name, err)
	}
	if obs.Story.Has(spec.Progress) {
		return nil
	}

	policy := skill.StatAwareMove(romData)
	err = skill.GoTo(m, romData, spec.Destination)
	if err != nil && !errors.Is(err, skill.ErrBattleInterrupted) &&
		!errors.Is(err, skill.ErrDialogueInterrupted) && !errors.Is(err, skill.ErrBattle) {
		return fmt.Errorf("%s: reach trigger: %w", spec.Name, err)
	}

	start := m.FrameCount()
	idle := 0
	for int(m.FrameCount()-start) <= spec.MaxFrames {
		obs, err = ObserveChecked(m, romData)
		if err != nil {
			return fmt.Errorf("%s: observe: %w", spec.Name, err)
		}
		if obs.Story.Has(spec.Progress) && obs.Controllable && !obs.InBattle {
			return nil
		}
		if obs.InBattle {
			idle = 0
			if _, err := skill.Battle(m, policy); err != nil {
				return fmt.Errorf("%s: scripted battle: %w", spec.Name, err)
			}
			continue
		}
		if !obs.Controllable {
			idle = 0
			if spec.AdvanceA {
				m.Tap(emu.A, 3, 7)
			} else {
				m.StepFrame()
			}
			continue
		}

		// Coordinate-triggered scripts can start a few frames after GoTo
		// reaches its boundary. Give them a bounded grace period, but fail
		// instead of blindly pressing input once normal control is stable.
		idle++
		if idle >= spec.IdleFrames {
			return fmt.Errorf("%s: trigger settled before progress %q committed on map %#02x at (%d,%d)",
				spec.Name, spec.Progress, obs.Map, obs.X, obs.Y)
		}
		m.StepFrame()
	}
	return fmt.Errorf("%s: exceeded %d frames before progress %q committed", spec.Name, spec.MaxFrames, spec.Progress)
}
