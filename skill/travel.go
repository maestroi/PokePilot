package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

type Replan struct {
	Map  uint8
	X, Y uint8
}

type TravelResult struct {
	Battles    int
	Flees      int
	Dialogues  int
	BlackedOut bool
	Replans    []Replan
}

// battleResolution records whether the fought interruption was a trainer.
// That fact is read before Battle clears wIsInBattle and survives long enough
// to classify a loss separately from a wild-battle or poison blackout.
type battleResolution struct {
	outcome state.BattleResult
	fled    bool
	trainer bool
}

type resolveBattle func() (battleResolution, error)

func fightOnly(m *emu.Emu, policy MovePolicy) resolveBattle {
	return func() (battleResolution, error) {
		trainer := state.BattleKind(m.Peek8(sym.IsInBattle)) == state.BattleTrainer
		outcome, err := Battle(m, policy)
		return battleResolution{outcome: outcome, trainer: trainer}, err
	}
}

func fleeThenFight(m *emu.Emu, policy MovePolicy, fleeAttempts int) resolveBattle {
	return func() (battleResolution, error) {
		if err := Flee(m, fleeAttempts); err != nil {
			if errors.Is(err, ErrTrainerBattle) {
				outcome, berr := Battle(m, policy)
				return battleResolution{outcome: outcome, trainer: true}, berr
			}
			return battleResolution{}, fmt.Errorf("skill: Travel: flee: %w", err)
		}
		return battleResolution{fled: true}, nil
	}
}

// ErrBlackedOut is the broad recovery class for any journey blackout.
var ErrBlackedOut = errors.New("skill: Travel: blacked out")

// ErrTrainerBlackedOut is the narrower recovery class for a lost mandatory
// trainer battle. It deliberately unwraps to ErrBlackedOut, so every existing
// caller using errors.Is(err, ErrBlackedOut) keeps its current recovery
// behavior while the agent can additionally learn that the same trainer is
// likely to beat the unchanged party again.
var ErrTrainerBlackedOut = fmt.Errorf("%w: lost trainer battle", ErrBlackedOut)

func battleBlackoutError(r battleResolution) error {
	if r.trainer {
		return ErrTrainerBlackedOut
	}
	return ErrBlackedOut
}

const blackoutBit = 1 << 5

type ErrDialogueChoice struct {
	Result DialogueRecoveryResult
}

func (e *ErrDialogueChoice) Error() string {
	return fmt.Sprintf("skill: Travel: text box is a choice and is unanswered: %q", e.Result.Text)
}

const (
	worldStableBudget = 1200
	worldStableFrames = 100
)

const maxDialogueRecoveries = 10
const maxRouteCuts = 4

func cutRecoverableNavigationError(err error) bool {
	return errors.Is(err, ErrLegUnwalkable) ||
		errors.Is(err, ErrReplanExhausted) ||
		errors.Is(err, world.ErrNoPath)
}

func cutAwareGoTo(m *emu.Emu, romData []byte, dest Destination) func() error {
	cuts := 0
	return func() error {
		for {
			err := GoTo(m, romData, dest)
			if err == nil || errors.Is(err, ErrBattle) || errors.Is(err, ErrDialogueInterrupted) {
				return err
			}
			if cuts >= maxRouteCuts || !cutRecoverableNavigationError(err) {
				return err
			}
			opened, cutErr := cutThroughReachableTree(m, romData)
			if cutErr != nil {
				if errors.Is(cutErr, ErrBattle) || errors.Is(cutErr, ErrDialogueInterrupted) {
					return cutErr
				}
				return fmt.Errorf("skill: Travel: Cut recovery after %v: %w", err, cutErr)
			}
			if !opened {
				return err
			}
			cuts++
		}
	}
}

// Travel fights every interrupting encounter. A lost trainer returns the
// narrow ErrTrainerBlackedOut while remaining an ErrBlackedOut for existing
// callers; a lost wild battle or poison wipe returns only ErrBlackedOut.
func Travel(m *emu.Emu, romData []byte, dest Destination, policy MovePolicy, maxBattles int) (TravelResult, error) {
	if maxBattles <= 0 {
		return TravelResult{}, fmt.Errorf("skill: Travel: maxBattles must be > 0, got %d", maxBattles)
	}
	return travel(m, policy, maxBattles,
		cutAwareGoTo(m, romData, dest),
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		fightOnly(m, policy),
	)
}

// TravelFlee flees wild encounters and fights trainers, which cannot be
// fled. Trainer losses use ErrTrainerBlackedOut just like Travel.
func TravelFlee(m *emu.Emu, romData []byte, dest Destination, policy MovePolicy, maxBattles int) (TravelResult, error) {
	return travel(m, policy, maxBattles,
		cutAwareGoTo(m, romData, dest),
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		fleeThenFight(m, policy, 5),
	)
}

func travel(m *emu.Emu, policy MovePolicy, maxBattles int, goTo func() error, recoverBox func() DialogueRecoveryResult, blackout func() bool, resolveBattle resolveBattle) (TravelResult, error) {
	var res TravelResult
	for {
		err := goTo()
		if err == nil {
			return res, nil
		}
		switch {
		case errors.Is(err, ErrBattle):
			if res.Battles+res.Flees >= maxBattles {
				return res, fmt.Errorf("skill: Travel: still interrupted after %d engagement(s) (maxBattles): %v",
					maxBattles, err)
			}
			pre := currentWorld(m)
			r, berr := resolveBattle()
			if berr != nil {
				return res, fmt.Errorf("skill: Travel: battle %d: %w", res.Battles+res.Flees+1, berr)
			}
			if r.fled {
				res.Flees++
			} else {
				res.Battles++
			}
			lost := r.outcome == state.ResultLost
			res.Replans = append(res.Replans, settleWorld(m, pre, lost))
			if lost {
				res.BlackedOut = true
				return res, battleBlackoutError(r)
			}
		case errors.Is(err, ErrDialogueInterrupted):
			if res.Dialogues >= maxDialogueRecoveries {
				return res, fmt.Errorf("skill: Travel: still interrupted by a text box after %d recoveries: %v",
					maxDialogueRecoveries, err)
			}
			res.Dialogues++
			rec := recoverBox()
			switch rec.Stop {
			case DialogueChoiceRequired, DialogueMenuOpen:
				return res, &ErrDialogueChoice{Result: rec}
			case DialogueBudgetExhausted:
				return res, fmt.Errorf("skill: Travel: text box did not clear within the recovery budget: %q", rec.Text)
			case DialogueRecovered:
				if blackout() {
					res.BlackedOut = true
					return res, ErrBlackedOut
				}
			case DialogueUnexpectedMode:
				// The next GoTo observes the battle and returns ErrBattle.
			}
		default:
			return res, err
		}
	}
}

func currentWorld(m *emu.Emu) Replan {
	return Replan{m.Peek8(sym.CurMap), m.Peek8(sym.XCoord), m.Peek8(sym.YCoord)}
}

func settleWorld(m *emu.Emu, pre Replan, lost bool) Replan {
	if lost {
		if _, err := m.StepUntil(worldStableBudget, func(m *emu.Emu) bool {
			return m.Peek8(sym.CurMap) != pre.Map
		}); err != nil {
			// Preserve the previous best-effort behavior on a slow transition.
		}
	}
	last := currentWorld(m)
	stable := 0
	for i := 0; i < worldStableBudget; i++ {
		m.StepFrame()
		cur := currentWorld(m)
		if cur == last {
			stable++
			if stable >= worldStableFrames {
				return cur
			}
		} else {
			stable = 0
		}
		last = cur
	}
	return last
}
