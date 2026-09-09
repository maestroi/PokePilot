package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

const objectiveFrameBudget uint64 = 500_000

type redObjectiveAdapter struct {
	m       *emu.Emu
	romData []byte
}

func newRedObjectiveAdapter(m *emu.Emu, romData []byte) *redObjectiveAdapter {
	return &redObjectiveAdapter{m: m, romData: romData}
}

func (a *redObjectiveAdapter) Observe() Observation {
	obs := Observe(a.m, a.romData)
	var mem state.Mem
	state.Snapshot(a.m, &mem)
	inv := state.DecodeInventory(&mem)
	obs.Story = redProgressStateFromRAM(&mem, inv, state.DecodeStoryFacts(&mem, inv))
	return obs
}

func (a *redObjectiveAdapter) Validate(o Objective, _ Observation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	switch o.Kind {
	case KindGoTo:
		if _, ok := skill.Place(o.Place); !ok {
			return fmt.Errorf("agent: %s: unknown Red place %q", o, o.Place)
		}
	case KindHeal:
		if o.Place != "" {
			d, ok := skill.Place(o.Place)
			if !ok {
				return fmt.Errorf("agent: %s: unknown Red place %q", o, o.Place)
			}
			if !isCenter(state.MapName(d.Map)) {
				return fmt.Errorf("agent: %s: %q is not a Pokemon Center", o, o.Place)
			}
		}
	case KindStarter:
		if o.Starter > skill.StarterBulbasaur {
			return fmt.Errorf("agent: %s: unsupported Red starter %d", o, int(o.Starter))
		}
	case KindProgress:
		if !redProgressionKnown(o.Progress) {
			return fmt.Errorf("agent: %s: unknown Red progression goal %q", o, o.Progress)
		}
	case KindCatch:
		if _, ok := redSpeciesID(o.Species); !ok {
			return fmt.Errorf("agent: %s: unknown Red species %q", o, o.Species)
		}
	case KindPickup, KindUseItem, KindBuy:
		if _, ok := a.resolveItemID(o.Item); !ok {
			return fmt.Errorf("agent: %s: unknown Red item %q", o, o.Item)
		}
	}
	return nil
}

func (a *redObjectiveAdapter) resolveItemID(id ItemID) (uint8, bool) {
	if raw, ok := redItemID(id); ok {
		return raw, true
	}
	var mem state.Mem
	state.Snapshot(a.m, &mem)
	inv := state.DecodeInventory(&mem)
	for _, item := range inv.Items {
		machine, err := rom.LookupTMHM(a.romData, item.ID)
		if err != nil {
			continue
		}
		if machineItemID(machine) == id {
			return item.ID, true
		}
	}
	return 0, false
}

func (a *redObjectiveAdapter) NormalizeBoundary() error {
	return normalizeObjectiveBoundary(a.m)
}

func (a *redObjectiveAdapter) ExecuteOwned(o Objective) (ObjectiveResult, error) {
	return executeRedOwned(a.m, a.romData, o)
}

func (a *redObjectiveAdapter) WithinObjectiveBudget(o Objective, fn func() error) error {
	deadline := a.m.FrameCount() + objectiveFrameBudget
	err := a.m.WithFrameDeadline(deadline, fn)
	if errors.Is(err, emu.ErrFrameDeadline) {
		return fmt.Errorf("agent: %s: objective frame watchdog: %w", o, err)
	}
	return err
}

func (a *redObjectiveAdapter) SettlePostcondition(o Objective) {
	settleObjectivePostcondition(a.m, o)
}

func (a *redObjectiveAdapter) VerifyPostcondition(o Objective, final Observation, _ ObjectiveResult) error {
	_, err := objectivePostcondition(o, final)
	if o.Kind == KindGoTo && errors.Is(err, ErrObjectivePostconditionFailed) {
		dest, ok := skill.Place(o.Place)
		if ok {
			var mem state.Mem
			state.Snapshot(a.m, &mem)
			if redOccupiedDestinationArrival(final, dest, state.DecodeSprites(&mem)) {
				return nil
			}
		}
	}
	return err
}

func (a *redObjectiveAdapter) CaptureFailure(o Objective, err error) error {
	return captureObjectiveFailure(a.m, o, err)
}

func executeObjective(m *emu.Emu, romData []byte, o Objective) (ObjectiveResult, error) {
	return Execute(m, romData, o)
}

// redOccupiedDestinationArrival mirrors GoTo's adjacent-arrival fallback.
// Only current sprite RAM is evidence; static object homes are not occupancy.
func redOccupiedDestinationArrival(final Observation, dest skill.Destination, sprites []state.SpriteState) bool {
	if !final.Controllable || final.InBattle || final.Map != dest.Map {
		return false
	}
	dx, dy := int(final.X)-int(dest.X), int(final.Y)-int(dest.Y)
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx+dy != 1 {
		return false
	}
	for _, sprite := range sprites {
		if sprite.X == int(dest.X) && sprite.Y == int(dest.Y) {
			return true
		}
	}
	return false
}
