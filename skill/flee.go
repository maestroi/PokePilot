package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// ErrTrainerBattle reports that the active game's RUN action was positively
// refused because the encounter is trainer-owned. The battle remains in
// progress so callers can hand it to Battle.
var ErrTrainerBattle = errors.New("skill: cannot flee a trainer battle")

// ErrFleeExhausted reports that every bounded RUN attempt completed without
// escaping and the encounter is still live. Callers that own the encounter
// may hand it to Battle rather than returning a poisoned in-battle boundary.
var ErrFleeExhausted = errors.New("skill: flee attempts exhausted")

// fleeAttemptBudget bounds one RUN attempt: refusal/escape text, the enemy's
// turn after a failed attempt, and any faint-and-switch episode in between.
const fleeAttemptBudget = 6000

type fleeControllers struct {
	escape    game.BattleEscapeMenuDecoder
	execution game.BattleExecutionDecoder
	runtime   game.BattleRuntimeDecoder
	resources game.BattleResourcesDecoder
	menu      game.MenuDecoder
	party     game.PartyMenuDecoder
}

func fleeControllersFor(m *emu.Emu) (fleeControllers, error) {
	escape, err := battleEscapeMenuDecoderFor(m)
	if err != nil {
		return fleeControllers{}, err
	}
	execution, err := battleExecutionDecoderFor(m)
	if err != nil {
		return fleeControllers{}, err
	}
	runtime, err := battleRuntimeDecoderFor(m)
	if err != nil {
		return fleeControllers{}, err
	}
	resources, err := battleResourcesDecoderFor(m)
	if err != nil {
		return fleeControllers{}, err
	}
	menu, err := menuDecoderFor(m)
	if err != nil {
		return fleeControllers{}, err
	}
	party, err := partyMenuDecoderFor(m)
	if err != nil {
		return fleeControllers{}, err
	}
	return fleeControllers{
		escape:    escape,
		execution: execution,
		runtime:   runtime,
		resources: resources,
		menu:      menu,
		party:     party,
	}, nil
}

// fleeMenuWait is the outcome of waiting for a battle's RUN menu. Kind names
// the RUN-capable menu once it is rendered; BattleOver reports that the
// encounter resolved before any RUN menu appeared, which is the only other way
// the wait can end.
type fleeMenuWait struct {
	Kind       game.BattleEscapeMenuKind
	BattleOver bool
}

// fleeWaitMachine is the execution surface a RUN-menu wait needs: the generic
// menu machine plus the frame counter its budget is measured in.
type fleeWaitMachine interface {
	menuMachine
	FrameCount() uint64
}

// waitFleeMenuWithControllers waits for the RUN-capable menu of a live battle.
// The battle ending is part of the exit condition because a menu belongs to
// its battle: Gen I's scripted encounters (the Old Man's catch demo, Prof.
// Oak's Pikachu) resolve themselves without ever offering the player a RUN
// menu, and their command menu is drawn with a stale wMaxMenuItem, so the
// escape decoder deliberately does not recognize it — recognizing it would
// only drive a cursor the ROM is steering itself (triage:cfe63dc059a15aa5).
// Waiting instead for a menu that cannot come spent all 3000 frames of
// run-1wsyy1f75ssxsheu3o4xpui4 and then failed a Flee whose battle had already
// ended 1000 frames into the wait.
func waitFleeMenuWithControllers(m fleeWaitMachine, controllers fleeControllers) (fleeMenuWait, error) {
	start := m.FrameCount()
	for {
		if !controllers.runtime.DecodeBattleRuntime(m).InBattle {
			return fleeMenuWait{BattleOver: true}, nil
		}
		if live := controllers.escape.DecodeBattleEscapeMenu(m); live.Visible {
			return fleeMenuWait{Kind: live.Kind}, nil
		}
		if int(m.FrameCount()-start) > bagMainMenuBudget {
			return fleeMenuWait{}, fmt.Errorf(
				"skill: Flee: battle RUN menu did not open within %d frames: %w",
				bagMainMenuBudget, ErrMenuStuck,
			)
		}

		execution := controllers.execution.DecodeBattleExecution(m)
		switch execution.Phase {
		case game.BattleExecutionMoveMenu, game.BattleExecutionMoveDisabled:
			m.Tap(emu.B, 3, 7)
		default:
			m.Tap(emu.A, 3, 7)
		}
	}
}

// Flee attempts to escape a battle through the active profile's semantic RUN
// menu. Failed wild escapes are retried up to attempts times. Success is
// positive: the profile must report the battle ended and settleAfterBattle
// must observe a stable controllable overworld boundary. A battle that
// resolves itself before it offers a RUN menu is the same success, because the
// encounter's end is what the caller asked for, not the RUN press.
func Flee(m *emu.Emu, attempts int) error {
	if attempts <= 0 {
		return fmt.Errorf("skill: Flee: attempts must be > 0, got %d", attempts)
	}
	controllers, err := fleeControllersFor(m)
	if err != nil {
		return fmt.Errorf("skill: Flee: %w", err)
	}
	live := controllers.runtime.DecodeBattleRuntime(m)
	if !live.InBattle {
		return fmt.Errorf("skill: Flee: no battle in progress on %s", battleRuntimeContext(live))
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		outcome, err := fleeOneAttempt(m, controllers)
		if err != nil {
			return err
		}
		if outcome == fleeSucceeded {
			return nil
		}
	}
	live = controllers.runtime.DecodeBattleRuntime(m)
	return fmt.Errorf("skill: Flee: still in battle after %d attempts: %s: %w", attempts, battleRuntimeContext(live), ErrFleeExhausted)
}

type fleeOutcome int

const (
	fleeSucceeded fleeOutcome = iota
	fleeFailed
)

func fleeOneAttempt(m *emu.Emu, controllers fleeControllers) (fleeOutcome, error) {
	wait, err := waitFleeMenuWithControllers(m, controllers)
	if err != nil {
		return 0, err
	}
	if wait.BattleOver {
		// No RUN decision ever existed; the encounter is already over, so the
		// remaining obligation is the settled overworld boundary.
		return fleeSucceeded, settleAfterBattle(m, controllers.runtime)
	}
	if err := selectBattleEscapeRunWithDecoder(m, controllers.escape); err != nil {
		return 0, fmt.Errorf("skill: Flee: select RUN: %w", err)
	}
	m.Tap(emu.A, 3, 7)

	start := m.FrameCount()
	refused := false
	forcedChoiceRounds := 0
	for int(m.FrameCount()-start) < fleeAttemptBudget {
		runtime := controllers.runtime.DecodeBattleRuntime(m)
		if !runtime.InBattle {
			return fleeSucceeded, settleAfterBattle(m, controllers.runtime)
		}

		execution := controllers.execution.DecodeBattleExecution(m)
		party := controllers.party.DecodePartyMenu(m)
		escape := controllers.escape.DecodeBattleEscapeMenu(m)

		switch {
		case !refused && execution.Phase == game.BattleExecutionRunRefused:
			refused = true
			m.Tap(emu.A, 3, 7)

		case execution.Phase == game.BattleExecutionSwitchBox:
			forcedChoiceRounds++
			if forcedChoiceRounds > forcedChoiceCap {
				return 0, fmt.Errorf("skill: Flee: %s: %w", battleRuntimeContext(runtime), ErrForcedChoiceStuck)
			}
			m.Tap(emu.B, 3, 7)

		case party.Visible && party.Kind == game.PartyMenuVoluntaryBattle:
			if forcedChoiceRounds == 0 {
				slot := controllers.resources.DecodeBattleResources(m).FirstLivePartySlot()
				if slot < 0 {
					m.StepFrame()
					continue
				}
				if err := selectPartySlotWithDecoder(m, controllers.party, slot); err != nil {
					return 0, fmt.Errorf("skill: Flee: select party slot (forced choice): %w", err)
				}
			} else {
				m.Tap(emu.B, 3, 7)
			}

		case escape.Visible:
			if refused {
				return 0, fmt.Errorf("skill: Flee: %s: %w", battleRuntimeContext(runtime), ErrTrainerBattle)
			}
			return fleeFailed, nil

		case execution.Phase == game.BattleExecutionUseNextPrompt:
			if _, ready := controllers.menu.DecodeTwoOption(m); !ready {
				m.Tap(emu.A, 3, 7)
				continue
			}
			if err := selectTwoOptionWithDecoder(m, controllers.menu, 0); err != nil {
				return 0, fmt.Errorf("skill: Flee: answer two-option prompt: %w", err)
			}

		case party.Visible && party.Kind == game.PartyMenuForcedBattle:
			slot := controllers.resources.DecodeBattleResources(m).FirstLivePartySlot()
			if slot < 0 {
				m.StepFrame()
				continue
			}
			if err := selectPartySlotWithDecoder(m, controllers.party, slot); err != nil {
				return 0, fmt.Errorf("skill: Flee: select party slot: %w", err)
			}

		default:
			m.Tap(emu.A, 3, 7)
		}
	}
	live := controllers.runtime.DecodeBattleRuntime(m)
	return 0, fmt.Errorf("skill: Flee: attempt did not resolve within %d frames: %s", fleeAttemptBudget, battleRuntimeContext(live))
}
