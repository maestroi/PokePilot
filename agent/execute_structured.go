package agent

import (
	"errors"
	"fmt"

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

func travelEvidenceFromRed(travel skill.TravelResult) *TravelEvidence {
	return &TravelEvidence{
		Battles:    travel.Battles,
		Flees:      travel.Flees,
		Dialogues:  travel.Dialogues,
		BlackedOut: travel.BlackedOut,
		Replans:    len(travel.Replans),
	}
}

func battleEvidenceFromRed(result state.BattleResult) *BattleEvidence {
	name := "unknown"
	switch result {
	case state.ResultWon:
		name = "won"
	case state.ResultLost:
		name = "lost"
	case state.ResultDraw:
		name = "draw"
	}
	return &BattleEvidence{Result: name, Won: result == state.ResultWon}
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
			if o.RepelBeforeTravel {
				if _, repelErr := skill.UseBestRepel(m); repelErr != nil {
					return result, fmt.Errorf("agent: %s: prepare Repel for speedrun travel: %w", o, repelErr)
				}
			}
			travel, err = skill.TravelFlee(m, romData, dest, skill.StatAwareMove(romData), 40)
		} else {
			travel, err = skill.Travel(m, romData, dest, skill.StatAwareMove(romData), 40)
		}
		result.Travel = travelEvidenceFromRed(travel)
		if err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindTalk:
		if err := validateRedTalkObjective(romData, m.Peek8(sym.CurMap), o); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		presses, err := skill.TalkAt(m, romData, o.X, o.Y, skill.StatAwareMove(romData))
		result.InteractionPresses = presses
		if err != nil {
			var menuErr *skill.ErrTalkMenu
			if errors.As(err, &menuErr) {
				declined, declineErr := declineUnexpectedGenericTalkChoice(m)
				if declineErr != nil {
					return result, fmt.Errorf("agent: %s: recover unexpected generic talk choice: %w", o, declineErr)
				}
				if declined {
					return result, nil
				}
			}
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil

	case KindTrainer:
		if err := skill.ChallengeTrainer(m, romData, o.X, o.Y, skill.StatAwareMove(romData)); err != nil {
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
		if o.Intent == "dex-evolution" {
			return executeDexEvolutionTraining(m, romData, o, result)
		}
		return executeTrainingObjective(m, romData, o, result)

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
				travel, err = skill.TravelFlee(m, romData, dest, skill.StatAwareMove(romData), 40)
			} else {
				travel, err = skill.Travel(m, romData, dest, skill.StatAwareMove(romData), 40)
			}
			result.Travel = travelEvidenceFromRed(travel)
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
		result.Battle = battleEvidenceFromRed(gym)
		if gym == state.ResultWon {
			return result, nil
		}
		result.Outcome = OutcomeBlocked
		return result, gymOutcomeErr(o, gym)

	case KindCatch:
		return executeCatchObjective(m, romData, o, result)

	case KindPickup:
		item, ok := adapter.resolvePickupItemID(m.Peek8(sym.CurMap), o)
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
		if o.Intent == speedrunRepelUseIntent {
			if err := skill.UseRepel(m, item); err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			result.ItemEffectVerified = true
			return result, nil
		}
		if o.Intent == "dex-evolution" {
			used, err := executeDexEvolutionItem(m, romData, o, result)
			if err == nil {
				used.ItemEffectVerified = true
			}
			return used, err
		}
		if _, err := rom.LookupTMHM(romData, item); err == nil {
			if _, err := skill.TeachTMHMToSlot(m, item, false, o.Slot); err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			result.ItemEffectVerified = true
			return result, nil
		}
		if err := skill.UseFieldItem(m, item, o.Slot); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		result.ItemEffectVerified = true
		return result, nil

	case KindBuy:
		item, ok := adapter.resolveItemID(o.Item)
		if !ok {
			return result, fmt.Errorf("agent: %s: unknown Red item %q", o, o.Item)
		}
		if o.Intent == dexEvolutionSupplyIntent {
			if o.Qty != 1 {
				return result, fmt.Errorf("agent: %s: evolution supply purchase must request exactly one stone", o)
			}
			travel, err := skill.BuyEvolutionStone(m, romData, item, skill.StatAwareMove(romData))
			result.Travel = travelEvidenceFromRed(travel)
			if err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			return result, nil
		}
		if err := skill.Buy(m, item, o.Qty); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	}
	return result, fmt.Errorf("agent: unknown objective kind %d", int(o.Kind))
}

func settleObjectivePostcondition(m *emu.Emu, o Objective) error {
	if o.Kind != KindGoTo {
		return nil
	}
	ready := func(em *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(em, &mem)
		return state.Controllable(&mem) && state.DecodeBattle(&mem) == nil
	}
	if ready(m) {
		return nil
	}
	if _, err := m.StepUntil(objectivePostconditionSettleBudget, ready); err != nil {
		// StepUntil checks before each frame, so the predicate may become true
		// on the final stepped frame. Re-check once before reporting timeout.
		if ready(m) {
			return nil
		}
		return fmt.Errorf("postcondition state did not settle: %w", err)
	}
	return nil
}
