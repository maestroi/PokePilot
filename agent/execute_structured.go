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

// objectivePostconditionSettleBudget is a passive settle only: it never presses
// input. A successful skill may finish a few frames before control returns, but
// an objective is not allowed to manufacture success by advancing dialogue or
// answering a choice while checking its postcondition.
const objectivePostconditionSettleBudget = 1200

// Execute carries out one objective and returns the structured facts produced
// by the owning skill. The low-level error remains the second return value so
// errors.Is/errors.As keep working; ObjectiveResult is the planner/run-facing
// semantic channel and therefore never contains an error interface.
//
// Skills retain their own positive assertions. Execute adds only the semantic
// boundary they cannot know, most importantly exact KindGoTo arrival. Claimed
// success with a false postcondition is terminal postcondition_failed; a state
// that is still impossible to check after the passive settle budget is terminal
// postcondition_unavailable.
func Execute(m *emu.Emu, romData []byte, o Objective) (result ObjectiveResult, retErr error) {
	result.Objective = o
	if err := o.Validate(); err != nil {
		result = finalizeObjectiveResult(o, result, Observe(m, romData), err)
		return result, err
	}

	// Preserve exact RAM at the public objective-error boundary before Run can
	// perform any later boundary recovery. Validation errors above are excluded
	// because no gameplay input was sent.
	defer func() {
		if retErr == nil && (result.Outcome == "" || result.Outcome == OutcomeCompleted) {
			settleObjectivePostcondition(m, o)
			final := Observe(m, romData)
			out, postErr := objectivePostcondition(o, final)
			if postErr != nil {
				result.Outcome = out
				retErr = fmt.Errorf("agent: %s: %w", o, postErr)
			}
			result = finalizeObjectiveResult(o, result, final, retErr)
		} else {
			result = finalizeObjectiveResult(o, result, Observe(m, romData), retErr)
		}
		if retErr != nil {
			if err := captureObjectiveFailure(m, o, retErr); err != nil {
				fmt.Printf("  ram forensics: %v\n", err)
			}
		}
	}()

	switch o.Kind {
	case KindGoTo:
		dest, ok := skill.Place(o.Place)
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
		if err := skill.GetStarter(m, romData, o.Starter, skill.StatAwareMove(romData)); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindErrand:
		if err := skill.OaksParcel(m, romData, skill.StatAwareMove(romData)); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindTrain:
		train, err := skill.Train(m, romData, int(o.Level), skill.StatAwareMove(romData), 20)
		result.Train = &train
		if err != nil {
			return result, fmt.Errorf("agent: %s: train failed after %d battles: %w", o, train.Battles, err)
		}
		if train.Reached {
			return result, nil
		}

		// These are legitimate bounded endings. They return an error for the
		// typed diagnostic channel but carry OutcomeBlocked explicitly, so Run
		// never has to reverse-engineer their meaning from prose.
		result.Outcome = OutcomeBlocked
		switch {
		case train.Retreated:
			return result, fmt.Errorf("agent: %s: %w (ended level %d)", o, skill.ErrTrainRetreat, train.EndLevel)
		case train.BlackedOut:
			return result, fmt.Errorf("agent: %s: %w before reaching level %d (ended level %d after %d battles)",
				o, skill.ErrBlackedOut, o.Level, train.EndLevel, train.Battles)
		case train.EndLevel > train.StartLevel:
			return result, fmt.Errorf("agent: %s: %w (target %d, ended level %d after %d battles)",
				o, skill.ErrTrainProgress, o.Level, train.EndLevel, train.Battles)
		default:
			return result, fmt.Errorf("agent: %s: target level %d not reached (ended level %d after %d battles)",
				o, o.Level, train.EndLevel, train.Battles)
		}

	case KindHeal:
		if o.Place != "" {
			dest, ok := skill.Place(o.Place)
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
		caught, err := skill.Catch(m, romData, []uint8{o.Species}, skill.StatAwareMove(romData), 5)
		if err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		if caught.Outcome == skill.OutcomeCaught {
			return result, nil
		}
		result.Outcome = OutcomeBlocked
		name, _ := SpeciesName(o.Species)
		return result, fmt.Errorf("agent: %s: no %s caught (outcome %s, balls=%d, encounters=%d)",
			o, strings.ToUpper(name), catchOutcomeName(caught.Outcome), caught.BallsThrown, caught.Encounters)

	case KindPickup:
		if err := skill.Pickup(m, romData, o.X, o.Y, o.Item, skill.StatAwareMove(romData)); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindUseItem:
		// Planner-offered TM/HM objectives use the same public Execute contract
		// as medicine; the machine path is selected from the ROM rather than
		// being a hidden Run-only dispatch exception.
		if _, err := rom.LookupTMHM(romData, o.Item); err == nil {
			if _, err := skill.TeachTMHMToSlot(m, o.Item, false, o.Slot); err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			return result, nil
		}
		if err := skill.UseFieldItem(m, o.Item, o.Slot); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindRocketHideout:
		if err := skill.RocketHideout(m, romData, skill.StatAwareMove(romData)); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindPokemonTower:
		if err := skill.PokemonTower(m, romData, skill.StatAwareMove(romData)); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindFuchsiaProgression:
		if err := skill.FuchsiaProgression(m, romData, skill.StatAwareMove(romData)); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindBuy:
		if err := skill.Buy(m, o.Item, o.Qty); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	}
	return result, fmt.Errorf("agent: unknown objective kind %d", int(o.Kind))
}

// settleObjectivePostcondition passively waits for a successful GoTo to reach
// a readable overworld boundary. It intentionally does not recover dialogue or
// menus: doing so would make a postcondition check own gameplay input.
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
