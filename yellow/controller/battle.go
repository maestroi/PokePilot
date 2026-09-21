package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowBattleFrameBudget  = 60000
	yellowBattleSettleBudget = 6000
	yellowMenuSettleBudget   = 180
	yellowBattleMenuLeftX    = 0x09
	yellowBattleMenuRightX   = 0x0f
)

var (
	ErrBattleChoiceRequired = errors.New("yellow battle: unsupported choice requires explicit policy")
	ErrNoUsableBattleMove   = errors.New("yellow battle: active Pokémon has no usable move")
)

type BattleOutcome string

const (
	BattleOutcomeWon    BattleOutcome = "won"
	BattleOutcomeLost   BattleOutcome = "lost"
	BattleOutcomeCaught BattleOutcome = "caught"
	BattleOutcomeEnded  BattleOutcome = "ended"
)

type BattleResult struct {
	Outcome BattleOutcome
	Raw     uint8
}

type battlePhase uint8

const (
	battlePhaseText battlePhase = iota
	battlePhaseMainMenu
	battlePhaseMoveMenu
	battlePhaseUseNext
	battlePhaseTrainerSwitch
	battlePhaseLearnMove
	battlePhaseAbandonLearning
	battlePhaseForcedParty
	battlePhaseUnknownChoice
)

func battlePhaseFor(text string, maxMenu uint8, forcedParty bool) battlePhase {
	upper := strings.ToUpper(text)
	switch {
	case forcedParty && strings.Contains(upper, "CHOOSE"):
		return battlePhaseForcedParty
	case strings.Contains(upper, "TYPE/"):
		return battlePhaseMoveMenu
	case strings.Contains(upper, "FIGHT"):
		return battlePhaseMainMenu
	case strings.Contains(upper, "USE NEXT"):
		return battlePhaseUseNext
	case strings.Contains(upper, "CHANGE POK"):
		return battlePhaseTrainerSwitch
	case strings.Contains(upper, "ABANDON LEARNING"):
		return battlePhaseAbandonLearning
	case strings.Contains(upper, "TRYING TO LEARN"):
		return battlePhaseLearnMove
	case maxMenu == 1 && strings.Contains(upper, "YES") && strings.Contains(upper, "NO"):
		return battlePhaseUnknownChoice
	default:
		return battlePhaseText
	}
}

// Battle resolves one ordinary Yellow battle with a deterministic first-usable
// move policy. It is deliberately Yellow-owned: menu and battle state come
// only from yellow/sym. More advanced switching, item use, capture policy and
// move replacement can layer on this state machine without importing Red RAM.
//
// The controller is bounded, refuses unknown two-option choices, and only
// succeeds after the battle has ended and a stable semantic boundary returns.
func Battle(m *emu.Emu, romData []byte) (BattleResult, error) {
	if m == nil {
		return BattleResult{}, fmt.Errorf("yellow battle: nil emulator")
	}
	if m.Peek8(sym.IsInBattle) == 0 {
		return BattleResult{}, fmt.Errorf("yellow battle: no battle in progress")
	}

	start := m.FrameCount()
	lost := m.Peek8(sym.IsInBattle) == 0xff
	pendingLearnDecline := false

	for int(m.FrameCount()-start) <= yellowBattleFrameBudget {
		inBattle := m.Peek8(sym.IsInBattle)
		if inBattle == 0xff {
			lost = true
		}
		if inBattle == 0 {
			raw := m.Peek8(sym.BattleResult)
			if err := settleYellowBattleBoundary(m, romData); err != nil {
				return BattleResult{}, err
			}
			switch {
			case lost:
				return BattleResult{Outcome: BattleOutcomeLost, Raw: raw}, nil
			case raw == 0:
				return BattleResult{Outcome: BattleOutcomeWon, Raw: raw}, nil
			case raw == 2:
				return BattleResult{Outcome: BattleOutcomeCaught, Raw: raw}, nil
			default:
				return BattleResult{Outcome: BattleOutcomeEnded, Raw: raw}, nil
			}
		}

		text := screenText(m)
		phase := battlePhaseFor(text, m.Peek8(sym.MaxMenuItem), m.Peek8(sym.ForcePlayerToChooseMon) != 0)
		switch phase {
		case battlePhaseMainMenu:
			pendingLearnDecline = false
			if err := selectYellowBattleFight(m); err != nil {
				return BattleResult{}, err
			}
			m.Tap(emu.A, 3, 7)

		case battlePhaseMoveMenu:
			pendingLearnDecline = false
			slot := firstUsableYellowMove(m)
			if slot < 0 {
				return BattleResult{}, ErrNoUsableBattleMove
			}
			if err := selectYellowLinearMenuItem(m, slot+1); err != nil {
				return BattleResult{}, fmt.Errorf("yellow battle: select move slot %d: %w", slot, err)
			}
			m.Tap(emu.A, 3, 7)

		case battlePhaseUseNext:
			if m.Peek8(sym.MaxMenuItem) == 1 {
				if err := selectYellowTwoOption(m, false); err != nil {
					return BattleResult{}, fmt.Errorf("yellow battle: accept next Pokémon: %w", err)
				}
			} else {
				m.Tap(emu.A, 3, 7)
			}

		case battlePhaseTrainerSwitch:
			// Decline optional trainer switching. A forced replacement is
			// handled separately from the party menu.
			if m.Peek8(sym.MaxMenuItem) == 1 {
				if err := selectYellowTwoOption(m, true); err != nil {
					return BattleResult{}, fmt.Errorf("yellow battle: decline trainer switch: %w", err)
				}
			} else {
				m.Tap(emu.A, 3, 7)
			}

		case battlePhaseLearnMove:
			// With fewer than four moves the engine learns automatically.
			// Once a replacement choice appears, basic battle policy declines
			// it rather than deleting an unknown move.
			pendingLearnDecline = true
			if m.Peek8(sym.MaxMenuItem) == 1 {
				if err := selectYellowTwoOption(m, true); err != nil {
					return BattleResult{}, fmt.Errorf("yellow battle: decline move replacement: %w", err)
				}
			} else {
				m.Tap(emu.A, 3, 7)
			}

		case battlePhaseAbandonLearning:
			if m.Peek8(sym.MaxMenuItem) == 1 {
				if err := selectYellowTwoOption(m, false); err != nil {
					return BattleResult{}, fmt.Errorf("yellow battle: confirm abandon learning: %w", err)
				}
				pendingLearnDecline = false
			} else {
				m.Tap(emu.A, 3, 7)
			}

		case battlePhaseForcedParty:
			slot, err := firstLiveYellowPartySlot(m, romData)
			if err != nil {
				return BattleResult{}, err
			}
			if slot < 0 {
				m.StepFrame()
				continue
			}
			if err := selectYellowPartySlot(m, slot); err != nil {
				return BattleResult{}, fmt.Errorf("yellow battle: forced party slot %d: %w", slot, err)
			}
			m.Tap(emu.A, 3, 7)

		case battlePhaseUnknownChoice:
			if pendingLearnDecline {
				if err := selectYellowTwoOption(m, true); err != nil {
					return BattleResult{}, fmt.Errorf("yellow battle: decline pending move replacement: %w", err)
				}
				continue
			}
			return BattleResult{}, fmt.Errorf("%w: screen=%q", ErrBattleChoiceRequired, strings.Join(strings.Fields(text), " "))

		case battlePhaseText:
			// This is paging, not blind menu selection: all battle menus and
			// known choices above are classified before A can be emitted.
			m.Tap(emu.A, 3, 7)
		}
	}

	return BattleResult{}, fmt.Errorf(
		"yellow battle: exceeded %d frames map=%#02x battle=%#02x screen=%q",
		yellowBattleFrameBudget, m.Peek8(sym.CurMap), m.Peek8(sym.IsInBattle),
		strings.Join(strings.Fields(screenText(m)), " "))
}

func firstUsableYellowMove(m *emu.Emu) int {
	for slot := 0; slot < 4; slot++ {
		move := m.Peek8(sym.BattleMonMoves + uint16(slot))
		pp := m.Peek8(sym.BattleMonPP+uint16(slot)) & 0x3f
		if move != 0 && pp != 0 {
			return slot
		}
	}
	return -1
}

func firstLiveYellowPartySlot(m *emu.Emu, romData []byte) (int, error) {
	obs, err := yellowprofile.New().DecodeObservation(m, romData)
	if err != nil {
		return -1, fmt.Errorf("yellow battle: observe party: %w", err)
	}
	for slot, mon := range obs.Party {
		if mon.HP > 0 {
			return slot, nil
		}
	}
	return -1, nil
}

func selectYellowBattleFight(m *emu.Emu) error {
	return selectYellowBattleMainMenu(m, yellowBattleMenuLeftX, 0)
}

func selectYellowBattleMainMenu(m *emu.Emu, targetX, targetRow uint8) error {
	if targetX != yellowBattleMenuLeftX && targetX != yellowBattleMenuRightX {
		return fmt.Errorf("yellow battle: invalid main-menu target x=%#02x", targetX)
	}
	if targetRow > 1 {
		return fmt.Errorf("yellow battle: invalid main-menu target row=%d", targetRow)
	}
	for attempt := 0; attempt < 8; attempt++ {
		x := m.Peek8(sym.TopMenuItemX)
		row := m.Peek8(sym.CurrentMenuItem)
		if x == targetX && row == targetRow {
			return nil
		}

		var button emu.Button
		switch {
		case x != yellowBattleMenuLeftX && x != yellowBattleMenuRightX:
			return fmt.Errorf("yellow battle: unknown main-menu cursor x=%#02x row=%d", x, row)
		case x != targetX:
			if targetX == yellowBattleMenuLeftX {
				button = emu.Left
			} else {
				button = emu.Right
			}
		case row < targetRow:
			button = emu.Down
		default:
			button = emu.Up
		}
		if err := tapYellowCursor(m, button, x, row); err != nil {
			return err
		}
	}
	return fmt.Errorf("yellow battle: main-menu cursor did not reach x=%#02x row=%d", targetX, targetRow)
}

func selectYellowTwoOption(m *emu.Emu, no bool) error {
	target := uint8(0)
	if no {
		target = 1
	}
	if m.Peek8(sym.MaxMenuItem) != 1 {
		return fmt.Errorf("two-option menu max=%d, want 1", m.Peek8(sym.MaxMenuItem))
	}
	if err := selectYellowLinearMenuItem(m, int(target)); err != nil {
		return err
	}
	m.Tap(emu.A, 3, 7)
	return nil
}

func selectYellowPartySlot(m *emu.Emu, target int) error {
	if target < 0 || target >= int(m.Peek8(sym.PartyCount)) {
		return fmt.Errorf("party target %d outside party count %d", target, m.Peek8(sym.PartyCount))
	}
	return selectYellowLinearMenuItem(m, target)
}

func selectYellowLinearMenuItem(m *emu.Emu, target int) error {
	if target < 0 || target > 255 {
		return fmt.Errorf("menu target %d is invalid", target)
	}
	for attempt := 0; attempt < 16; attempt++ {
		cur := m.Peek8(sym.CurrentMenuItem)
		if int(cur) == target {
			return nil
		}
		button := emu.Down
		if int(cur) > target {
			button = emu.Up
		}
		if err := tapYellowCursor(m, button, m.Peek8(sym.TopMenuItemX), cur); err != nil {
			return err
		}
	}
	return fmt.Errorf("yellow menu cursor did not reach %d from %d", target, m.Peek8(sym.CurrentMenuItem))
}

func tapYellowCursor(m *emu.Emu, button emu.Button, oldX, oldItem uint8) error {
	m.Tap(button, 3, 7)
	if m.Peek8(sym.TopMenuItemX) != oldX || m.Peek8(sym.CurrentMenuItem) != oldItem {
		return nil
	}
	if _, err := m.StepUntil(yellowMenuSettleBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.TopMenuItemX) != oldX || m.Peek8(sym.CurrentMenuItem) != oldItem
	}); err != nil {
		return fmt.Errorf("yellow menu cursor stuck at x=%#02x item=%d: %w", oldX, oldItem, err)
	}
	return nil
}

func settleYellowBattleBoundary(m *emu.Emu, romData []byte) error {
	stable := 0
	for frame := 0; frame < yellowBattleSettleBudget; frame++ {
		obs, err := yellowprofile.New().DecodeObservation(m, romData)
		if err != nil {
			return fmt.Errorf("yellow battle: settle observation: %w", err)
		}
		if obs.Controllable && !obs.InBattle {
			stable++
			if stable >= 12 {
				return nil
			}
		} else {
			stable = 0
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("yellow battle: boundary did not settle within %d frames", yellowBattleSettleBudget)
}
