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
	return Observe(a.m, a.romData)
}

// Validate first enforces the portable Objective shape, then resolves semantic
// ids against Pokémon Red. Numeric ROM ids never need to cross the planner
// boundary merely so Red can reject an unsupported name.
func (a *redObjectiveAdapter) Validate(o Objective, _ Observation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	switch o.Kind {
	case KindGoTo:
		if _, ok := skill.Place(string(o.Place)); !ok {
			return fmt.Errorf("agent: %s: unknown Red place %q", o, o.Place)
		}
	case KindHeal:
		if o.Place != "" {
			d, ok := skill.Place(string(o.Place))
			if !ok {
				return fmt.Errorf("agent: %s: unknown Red place %q", o, o.Place)
			}
			if !isCenter(state.MapName(d.Map)) {
				return fmt.Errorf("agent: %s: %q is not a Pokemon Center", o, o.Place)
			}
		}
	case KindStarter:
		if _, ok := redStarter(o.Starter); !ok {
			return fmt.Errorf("agent: %s: unsupported Red starter %q", o, o.Starter)
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

// resolveItemID maps a semantic item back to Red. Ordinary planner vocabulary
// uses the static item table; TM/HM objectives may name machines that are only
// known from this ROM/inventory, so resolve those from live owned items rather
// than exposing their byte ids to the planner.
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
	return err
}

func (a *redObjectiveAdapter) CaptureFailure(o Objective, err error) error {
	return captureObjectiveFailure(a.m, o, err)
}

func executeObjective(m *emu.Emu, romData []byte, o Objective) (ObjectiveResult, error) {
	return Execute(m, romData, o)
}
