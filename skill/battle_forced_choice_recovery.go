package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const forcedChoiceRecoveryPasses = 6

// recoverForcedChoiceBattle is the bounded recovery path for a Battle that
// returned ErrForcedChoiceStuck while a trainer-refusal party menu was still
// on screen. Battle/menu/roster observations come from profile semantics.
func recoverForcedChoiceBattle(m *emu.Emu, policy MovePolicy) error {
	battleDecoder, err := battleStateDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: forced-choice recovery: %w", err)
	}
	executionDecoder, err := battleExecutionDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: forced-choice recovery: %w", err)
	}
	partyDecoder, err := partyMenuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: forced-choice recovery: %w", err)
	}
	resourcesDecoder, err := battleResourcesDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: forced-choice recovery: %w", err)
	}

	inBattle := func() bool {
		_, ok := battleDecoder.DecodeBattleState(m)
		return ok
	}
	mainMenu := func() bool {
		return executionDecoder.DecodeBattleExecution(m).Phase == game.BattleExecutionMainMenu
	}
	switchBox := func() bool {
		return executionDecoder.DecodeBattleExecution(m).Phase == game.BattleExecutionSwitchBox
	}
	voluntaryParty := func() bool {
		live := partyDecoder.DecodePartyMenu(m)
		return live.Visible && live.Kind == game.PartyMenuVoluntaryBattle
	}

	for pass := 0; pass < forcedChoiceRecoveryPasses; pass++ {
		if !inBattle() {
			return battleRecoveryOutcomeWithDecoder(m, battleDecoder)
		}

		switch {
		case mainMenu():
			outcome, err := Battle(m, policy)
			if err != nil {
				return fmt.Errorf("skill: forced-choice recovery: %w", err)
			}
			if outcome == game.BattleLost {
				return ErrBlackedOut
			}
			return nil

		case switchBox():
			m.Tap(emu.B, 3, 7)
			if _, err := m.StepUntil(moveMenuBudget, func(*emu.Emu) bool {
				return !switchBox() || !inBattle()
			}); err != nil {
				return forcedChoiceMenuError(m, "leave forced-choice SWITCH/STATS box", err)
			}

		case voluntaryParty():
			m.Tap(emu.B, 3, 7)
			if _, err := m.StepUntil(moveMenuBudget, func(*emu.Emu) bool {
				return !voluntaryParty() || !inBattle()
			}); err == nil {
				continue
			}

			slot := resourcesDecoder.DecodeBattleResources(m).FirstLivePartySlot()
			if slot < 0 {
				return fmt.Errorf("skill: forced-choice recovery: no live party member")
			}
			if err := selectPartySlotWithDecoder(m, partyDecoder, slot); err != nil {
				return forcedChoiceMenuError(m, "select live party slot during forced-choice recovery", err)
			}
			if _, err := m.StepUntil(moveMenuBudget, func(*emu.Emu) bool {
				return switchBox() || mainMenu() || !inBattle()
			}); err != nil {
				return forcedChoiceMenuError(m, "wait for forced-choice slot selection", err)
			}

		default:
			m.StepFrames(12)
		}
	}

	if !inBattle() {
		return battleRecoveryOutcomeWithDecoder(m, battleDecoder)
	}
	return fmt.Errorf("skill: forced-choice recovery did not reach the main battle menu after %d settled transitions: %w", forcedChoiceRecoveryPasses, ErrForcedChoiceStuck)
}

func forcedChoiceMenuError(m *emu.Emu, detail string, err error) error {
	_, wrapped := menuError(m, detail, err)
	return wrapped
}

func battleRecoveryOutcome(m *emu.Emu) error {
	decoder, err := battleStateDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: battle recovery outcome: %w", err)
	}
	return battleRecoveryOutcomeWithDecoder(m, decoder)
}

func battleRecoveryOutcomeWithDecoder(m *emu.Emu, decoder game.BattleStateDecoder) error {
	if decoder.DecodeBattleResult(m) == game.BattleLost {
		return ErrBlackedOut
	}
	return nil
}
