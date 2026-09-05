package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

const forcedChoiceRecoveryPasses = 6

// recoverForcedChoiceBattle is the bounded recovery path for a Battle that
// returned ErrForcedChoiceStuck while a trainer-refusal party menu was still
// on screen. The normal Battle loop already knows the right logical sequence;
// this helper adds the missing transition settling: every B/A is followed by
// a positive wait for the current menu to actually change before another
// input is sent.
//
// This matters on farm timing where the tile-map marker can remain visible for
// several frames after B. The old loop could count the same still-rendered
// party list as five separate visits, fire five B presses into one transition,
// then give up even though the ROM was making progress.
//
// The helper is intentionally narrow and bounded. It does not answer arbitrary
// battle menus. If B is still disabled on the forced party list, it selects a
// live slot once so the ROM can clear the forced-choice state, backs out of the
// SWITCH/STATS box, then exits the party list. Once the main battle menu is
// visible, Battle resumes ownership of the fight.
func recoverForcedChoiceBattle(m *emu.Emu, policy MovePolicy) error {
	for pass := 0; pass < forcedChoiceRecoveryPasses; pass++ {
		if !battleInFlight(m) {
			return battleRecoveryOutcome(m)
		}

		switch {
		case mainMenuUp(m):
			outcome, err := Battle(m, policy)
			if err != nil {
				return fmt.Errorf("skill: forced-choice recovery: %w", err)
			}
			if outcome == state.ResultLost {
				return ErrBlackedOut
			}
			return nil

		case switchBoxUp(m):
			m.Tap(emu.B, 3, 7)
			if _, err := m.StepUntil(moveMenuBudget, func(m *emu.Emu) bool {
				return !switchBoxUp(m) || !battleInFlight(m)
			}); err != nil {
				return menuError(m, "leave forced-choice SWITCH/STATS box", err)
			}

		case battleSwitchMenuUp(m):
			// B is the correct exit once PartyMenuInit has cleared the
			// force flag. Wait for the marker to disappear before deciding it
			// failed; a stale tile map must not become five synthetic visits.
			m.Tap(emu.B, 3, 7)
			if _, err := m.StepUntil(moveMenuBudget, func(m *emu.Emu) bool {
				return !battleSwitchMenuUp(m) || !battleInFlight(m)
			}); err == nil {
				continue
			}

			// B can legitimately be disabled on the first forced visit. Pick
			// one live slot to enter SWITCH/STATS/CANCEL; backing out of that
			// box re-runs PartyMenuInit with B enabled.
			var mem state.Mem
			state.Snapshot(m, &mem)
			slot := firstLivePartySlot(&mem)
			if slot < 0 {
				return fmt.Errorf("skill: forced-choice recovery: no live party member")
			}
			if err := SelectPartySlot(m, slot); err != nil {
				return menuError(m, "select live party slot during forced-choice recovery", err)
			}
			if _, err := m.StepUntil(moveMenuBudget, func(m *emu.Emu) bool {
				return switchBoxUp(m) || mainMenuUp(m) || !battleInFlight(m)
			}); err != nil {
				return menuError(m, "wait for forced-choice slot selection", err)
			}

		default:
			// A transition may be between recognizable tile-map states.
			// Let it settle instead of blindly pressing A or B into it.
			m.StepFrames(12)
		}
	}

	if !battleInFlight(m) {
		return battleRecoveryOutcome(m)
	}
	return fmt.Errorf("skill: forced-choice recovery did not reach the main battle menu after %d settled transitions: %w", forcedChoiceRecoveryPasses, ErrForcedChoiceStuck)
}

func battleRecoveryOutcome(m *emu.Emu) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattleResult(&mem) == state.ResultLost {
		return ErrBlackedOut
	}
	return nil
}
