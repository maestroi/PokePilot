package skill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ErrTrainerBattle reports that the battle is a trainer battle, which
// refuses the RUN option: the ROM prints "No! There's no running from a
// trainer battle!" (data/text/text_2.asm _NoRunningText) and sets
// wForcePlayerToChooseMon instead of rolling an escape (engine/battle/
// core.asm TryRunningFromBattle .trainerBattle). Flee returns it after the
// first refusal rather than burning all its attempts: the ROM refuses every
// one of them identically. The battle is still in progress when this is
// returned, so a caller can hand it to Battle.
var ErrTrainerBattle = errors.New("skill: cannot flee a trainer battle")

// fleeAttemptBudget bounds one RUN attempt: the refusal/escape text, the
// enemy's turn (which a failed attempt costs), and any faint-and-switch
// episode in between. The cap fails loudly instead of hanging, as in Battle.
const fleeAttemptBudget = 6000

// trainerNoRunningMarker is on NoRunningText ("No! There's no running from a
// trainer battle!") and on no other battle screen. It is the POSITIVE fact
// that the RUN option was refused because this is a trainer battle — not an
// inference from "nothing happened".
const trainerNoRunningMarker = "running from a"

// Flee attempts to escape a wild battle. A failed attempt is not an error:
// wNumRunAttempts (ram/wram.asm) improves the odds on each try within the
// same battle — core.asm TryRunningFromBattle adds 30 to the escape
// quotient per prior attempt — so Flee retries up to attempts times. The
// postcondition is POSITIVE — the battle is over (wIsInBattle clear, read
// back via DecodeBattle) and the player is controllable again; "the menu
// went away" is not enough, settleAfterBattle enforces it.
//
// A trainer battle cannot be fled: the first refusal returns ErrTrainerBattle
// with the battle still in progress. Flee never fights the battle itself.
func Flee(m *emu.Emu, attempts int) error {
	if attempts <= 0 {
		return fmt.Errorf("skill: Flee: attempts must be > 0, got %d", attempts)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) == nil {
		x, y := playerXY(m)
		return fmt.Errorf("skill: Flee: no battle in progress on map %02x at (%d,%d)", m.Peek8(sym.CurMap), x, y)
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		outcome, err := fleeOneAttempt(m)
		if err != nil {
			return err
		}
		if outcome == fleeSucceeded {
			return nil
		}
	}
	x, y := playerXY(m)
	return fmt.Errorf("skill: Flee: still in battle after %d attempts: map %02x at (%d,%d)", attempts, m.Peek8(sym.CurMap), x, y)
}

// fleeOutcome is what one RUN attempt ended in.
type fleeOutcome int

const (
	// fleeSucceeded: the escape took; the battle is over and the player is
	// controllable (settleAfterBattle already ran).
	fleeSucceeded fleeOutcome = iota
	// fleeFailed: "Can't escape!" resolved, the enemy took its turn, and the
	// FIGHT/ITEM/PKMN/RUN menu is up again. Retry with better odds.
	fleeFailed
)

// fleeOneAttempt drives one RUN selection and follows it to a resolution:
// success (battle over), a trainer refusal (typed error), or the main menu
// back up (retry). Every transition is read from RAM, never inferred from
// press counts.
func fleeOneAttempt(m *emu.Emu) (fleeOutcome, error) {
	// waitBattleMainMenu advances the encounter or "Can't escape!" text until
	// the FIGHT/ITEM/PKMN/RUN menu is drawn, stopping the moment it appears
	// so it never presses A on top of the menu itself.
	if err := waitBattleMainMenu(m); err != nil {
		return 0, err
	}
	if err := selectRunEntry(m); err != nil {
		return 0, err
	}
	m.Tap(emu.A, 3, 7)

	var mem state.Mem
	start := m.FrameCount()
	for int(m.FrameCount()-start) < fleeAttemptBudget {
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) == nil {
			// Escaped: the battle is over. Settle the end-of-battle text and
			// wait until the player is controllable — the positive half of
			// the postcondition.
			return fleeSucceeded, settleAfterBattle(m, &mem)
		}
		if strings.Contains(state.ScreenText(&mem), trainerNoRunningMarker) {
			x, y := playerXY(m)
			return 0, fmt.Errorf("skill: Flee: map %02x at (%d,%d): %w", m.Peek8(sym.CurMap), x, y, ErrTrainerBattle)
		}
		switch {
		case mainMenuUp(m):
			// "Can't escape!" resolved and the enemy took its turn: the menu
			// is up again. wNumRunAttempts already counts this attempt, so
			// the next roll has better odds.
			return fleeFailed, nil

		case twoOptionPromptUp(m):
			// The lead fainted on the enemy's turn after a failed attempt:
			// answer "Use next #MON?" YES, exactly as Battle does (NO is
			// itself an escape attempt).
			var s state.Mem
			state.Snapshot(m, &s)
			if state.DecodeTwoOptionMenu(&s) == nil {
				// The prompt text is on screen but the box is not drawn yet:
				// tap A to advance it and look again.
				m.Tap(emu.A, 3, 7)
				continue
			}
			if err := SelectMenuItem(m, 0); err != nil {
				return 0, fmt.Errorf("skill: Flee: answer two-option prompt: %w", err)
			}

		case partyMenuUp(m):
			// Forced switch after the faint: send out the first live slot,
			// the same pick Battle makes (the ROM bounces a fainted one).
			var s state.Mem
			state.Snapshot(m, &s)
			slot := firstLivePartySlot(&s)
			if slot < 0 {
				m.StepFrame()
				continue
			}
			if err := SelectPartySlot(m, slot); err != nil {
				return 0, fmt.Errorf("skill: Flee: select party slot: %w", err)
			}

		default:
			// Text or an animation (the escape sequence, the enemy's turn).
			// Advance it and look again.
			m.Tap(emu.A, 3, 7)
		}
	}
	x, y := playerXY(m)
	return 0, fmt.Errorf("skill: Flee: attempt did not resolve within %d frames: map %02x at (%d,%d)", fleeAttemptBudget, m.Peek8(sym.CurMap), x, y)
}

// selectRunEntry moves the cursor of the FIGHT/ITEM/PKMN/RUN menu to RUN —
// the last entry: right column, row mainMenuMax. The grid is 2 columns by
// mainMenuMax+1 rows with wMaxMenuItem == 1 per column, so SelectMenuItem
// cannot reach it; step-and-verify against wTopMenuItemX and
// wCurrentMenuItem instead, as SwitchActive does for POKéMON. The cursor
// opens at FIGHT but a stale saved item could leave it anywhere in the
// grid, so every tap is verified before the next one — never a press count.
func selectRunEntry(m *emu.Emu) error {
	atRun := func(m *emu.Emu) bool {
		return m.Peek8(sym.TopMenuItemX) == battleMenuRightX && int(m.Peek8(sym.CurrentMenuItem)) == mainMenuMax
	}
	for i := 0; i < 12; i++ {
		if atRun(m) {
			return nil
		}
		prevX, prevRow := m.Peek8(sym.TopMenuItemX), int(m.Peek8(sym.CurrentMenuItem))
		var btn emu.Button
		switch {
		case prevRow < mainMenuMax:
			btn = emu.Down // FIGHT -> ITEM and PKMN -> RUN
		case prevX == battleMenuLeftX:
			btn = emu.Right // ITEM -> RUN
		default:
			btn = emu.Left // right column below the target: back left
		}
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return m.Peek8(sym.TopMenuItemX) != prevX || int(m.Peek8(sym.CurrentMenuItem)) != prevRow
		}); err != nil {
			return fmt.Errorf("skill: Flee: cursor stuck at x=%#02x row %d, want RUN (x=%#02x row %d)", prevX, prevRow, battleMenuRightX, mainMenuMax)
		}
	}
	return fmt.Errorf("skill: Flee: cursor did not reach RUN")
}
