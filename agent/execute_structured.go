package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
)

const objectivePostconditionSettleBudget = 1200

func Execute(m *emu.Emu, romData []byte, o Objective) (ObjectiveResult, error) {
	return executeObjectiveWithAdapter(newRedObjectiveAdapter(m, romData), o)
}

// executeRedOwned is the Red adapter's action dispatcher. All semantic entity
// ids are translated here, immediately before a Red skill consumes its native
// numeric/index representation.
func executeRedOwned(m *emu.Emu, romData []byte, o Objective) (result ObjectiveResult, retErr error) {
	result.Objective = o
	adapter := newRedObjectiveAdapter(m, romData)

	switch o.Kind {
	case KindGoTo:
		dest, ok := skill.Place(string(o.Place))
		if !ok {
			return result, fmt.Errorf("agent: %s: unknown place %q", o, o.Place)
		}
		var (
			travel skill.TravelResult
			err    error
		)
		if o.Flee {
			travel, err = skill.TravelFlee(m, romData, dest, skill.StatAwareMove(romData), 20)
		} else {
			travel, err = skill.Travel(m, romData, dest, skill.StatAwareMove(romData), 20)
		}
		result.Travel = &travel
		if err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindTalk:
		if _, err := skill.TalkAt(m, romData, o.X, o.Y, skill.StatAwareMove(romData)); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindStarter:
		starter, ok := redStarter(o.Starter)
		if !ok {
			return result, fmt.Errorf("agent: %s: unsupported Red starter %q", o, o.Starter)
		}
		if err := skill.GetStarter(m, romData, starter, skill.StatAwareMove(romData)); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindProgress:
		if err := executeRedProgression(m, romData, o); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindTrain:
		if estimate, err := currentTrainingEstimateFromEmu(m, romData, m.Peek8(sym.CurMap), int(o.Level), trainSessionBattleBudget); err == nil && estimate.Viability == TrainingOutsideBudget {
			result.Outcome = OutcomeBlocked
			return result, fmt.Errorf("agent: %s: %w", o, &TrainingInefficientError{Estimate: estimate})
		}
		train, err := skill.Train(m, romData, int(o.Level), skill.StatAwareMove(romData), trainSessionBattleBudget)
		result.Train = &train
		if err != nil {
			return result, fmt.Errorf("agent: %s: train failed after %d battles: %w", o, train.Battles, err)
		}
		if train.Reached {
			return result, nil
		}
		result.Outcome = OutcomeBlocked
		switch {
		case train.Retreated:
			return result, fmt.Errorf("agent: %s: %w (ended level %d)", o, skill.ErrTrainRetreat, train.EndLevel)
		case train.BlackedOut:
			return result, fmt.Errorf("agent: %s: %w before reaching level %d (ended level %d after %d battles)", o, skill.ErrBlackedOut, o.Level, train.EndLevel, train.Battles)
		case train.EndLevel > train.StartLevel:
			return result, fmt.Errorf("agent: %s: %w (target %d, ended level %d after %d battles)", o, skill.ErrTrainProgress, o.Level, train.EndLevel, train.Battles)
		default:
			return result, fmt.Errorf("agent: %s: target level %d not reached (ended level %d after %d battles)", o, o.Level, train.EndLevel, train.Battles)
		}

	case KindHeal:
		if o.Place != "" {
			dest, ok := skill.Place(string(o.Place))
			if !ok {
				return result, fmt.Errorf("agent: %s: unknown place %q", o, o.Place)
			}
			var (
				travel skill.TravelResult
				err    error
			)
			if o.Flee {
				travel, err = skill.TravelFlee(m, romData, dest, skill.StatAwareMove(romData), 20)
			} else {
				travel, err = skill.Travel(m, romData, dest, skill.StatAwareMove(romData), 20)
			}
			result.Travel = &travel
			if err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
		}
		if dest, ok := skill.PlaceOnMap(m.Peek8(sym.CurMap)); ok {
			if err := skill.GoTo(m, romData, dest); err != nil {
				return result, fmt.Errorf("agent: %s: walk to the nurse: %w", o, err)
			}
		}
		if err := skill.Heal(m); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindGym:
		gym, err := skill.Gym(m, romData, skill.StatAwareMove(romData))
		if err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		result.GymOutcome = &gym
		if gym == state.ResultWon {
			return result, nil
		}
		result.Outcome = OutcomeBlocked
		return result, gymOutcomeErr(o, gym)

	case KindCatch:
		species, ok := redSpeciesID(o.Species)
		if !ok {
			return result, fmt.Errorf("agent: %s: unknown Red species %q", o, o.Species)
		}
		caught, err := skill.Catch(m, romData, []uint8{species}, skill.StatAwareMove(romData), 5)
		if err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		if caught.Outcome == skill.OutcomeCaught {
			return result, nil
		}
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: no %s caught (outcome %s, balls=%d, encounters=%d)", o, strings.ToUpper(string(o.Species)), catchOutcomeName(caught.Outcome), caught.BallsThrown, caught.Encounters)

	case KindPickup:
		item, ok := adapter.resolveItemID(o.Item)
		if !ok {
			return result, fmt.Errorf("agent: %s: unknown Red item %q", o, o.Item)
		}
		if err := skill.Pickup(m, romData, o.X, o.Y, item, skill.StatAwareMove(romData)); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindUseItem:
		item, ok := adapter.resolveItemID(o.Item)
		if !ok {
			return result, fmt.Errorf("agent: %s: unknown Red item %q", o, o.Item)
		}
		if _, err := rom.LookupTMHM(romData, item); err == nil {
			if _, err := skill.TeachTMHMToSlot(m, item, false, o.Slot); err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			return result, nil
		}
		if err := skill.UseFieldItem(m, item, o.Slot); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindBuy:
		item, ok := adapter.resolveItemID(o.Item)
		if !ok {
			return result, fmt.Errorf("agent: %s: unknown Red item %q", o, o.Item)
		}
		if err := skill.Buy(m, item, o.Qty); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	}
	return result, fmt.Errorf("agent: unknown objective kind %d", int(o.Kind))
}

func settleObjectivePostcondition(m *emu.Emu, o Objective) {
	if o.Kind != KindGoTo {
		return
	}
	ready := func(em *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(em, &mem)
		return state.Controllable(&mem) && state.DecodeBattle(&mem) == nil
	}
	if ready(m) {
		return
	}
	_, _ = m.StepUntil(objectivePostconditionSettleBudget, ready)
}
