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

func waitFleeMenuWithControllers(m *emu.Emu, controllers fleeControllers) (game.BattleEscapeMenuKind, error) {
	start := m.FrameCount()
	for {
		if live := controllers.escape.DecodeBattleEscapeMenu(m); live.Visible {
			return live.Kind, nil
		}
		if int(m.FrameCount()-start) > bagMainMenuBudget {
			return "", fmt.Errorf("skill: Flee: battle RUN menu did not open within %d frames", bagMainMenuBudget)
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
// must observe a stable controllable overworld boundary.
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
	return fmt.Errorf("skill: Flee: still in battle after %d attempts: %s", attempts, battleRuntimeContext(live))
}

type fleeOutcome int

const (
	fleeSucceeded fleeOutcome = iota
	fleeFailed
)

func fleeOneAttempt(m *emu.Emu, controllers fleeControllers) (fleeOutcome, error) {
	if _, err := waitFleeMenuWithControllers(m, controllers); err != nil {
		return 0, err
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
