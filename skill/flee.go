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

const (
	// trainerNoRunningMarker is on NoRunningText ("No! There's no running
	// from a trainer battle!") and on no other battle screen. It is the
	// POSITIVE fact that the RUN option was refused because this is a trainer
	// battle — not an inference from "nothing happened".
	trainerNoRunningMarker = "running from a"

	// Safari battles do not draw the ordinary FIGHT/ITEM/PKMN/RUN box.
	// DisplayBattleMenu selects SAFARI_BATTLE_MENU_TEMPLATE, whose second row
	// is "THROW ROCK  RUN" (pokered/data/text_boxes.asm). This marker is how
	// Flee knows which live menu it must drive; waiting only for FIGHT would
	// stall every Safari traversal on the first encounter.
	safariBattleMenuMarker = "THROW ROCK"

	// SAFARI_BATTLE_MENU_TEMPLATE prints its left text at x=2 and right text
	// at x=14, so HandleMenuInput stores cursor X one tile before those
	// columns: 1 and 13. RUN is the lower item in the right column.
	safariBattleMenuLeftX  byte = 0x01
	safariBattleMenuRightX byte = 0x0D
)

type fleeMenuKind uint8

const (
	fleeMenuNone fleeMenuKind = iota
	fleeMenuNormal
	fleeMenuSafari
)

// fleeMenuFromMem classifies only a fully rendered RUN-capable battle menu.
// Text/animations return fleeMenuNone, which keeps input away from a menu
// until its positive on-screen marker exists.
func fleeMenuFromMem(mem *state.Mem) fleeMenuKind {
	text := state.ScreenText(mem)
	switch {
	case strings.Contains(text, mainMenuMarker):
		return fleeMenuNormal
	case strings.Contains(text, safariBattleMenuMarker):
		return fleeMenuSafari
	default:
		return fleeMenuNone
	}
}

// safariRunCursor reports the exact live Safari menu state for RUN. It is
// kept separate from the emulator driver so ROM-free tests can pin the menu
// semantics directly.
func safariRunCursor(mem *state.Mem) bool {
	return fleeMenuFromMem(mem) == fleeMenuSafari &&
		mem.U8(sym.TopMenuItemX) == safariBattleMenuRightX &&
		int(mem.U8(sym.CurrentMenuItem)) == mainMenuMax
}

// safariRunNextInput returns the next verified cursor move toward RUN, or
// done=true when the cursor is already there. The Safari menu is the same
// two-column/two-row HandleMenuInput shape as the ordinary battle menu, but
// its columns are at different X coordinates.
func safariRunNextInput(mem *state.Mem) (btn emu.Button, done bool) {
	if safariRunCursor(mem) {
		return 0, true
	}
	row := int(mem.U8(sym.CurrentMenuItem))
	x := mem.U8(sym.TopMenuItemX)
	switch {
	case row < mainMenuMax:
		return emu.Down, false
	case row > mainMenuMax:
		return emu.Up, false
	case x < safariBattleMenuRightX:
		return emu.Right, false
	case x > safariBattleMenuRightX:
		return emu.Left, false
	default:
		// x is the right column and row is the lower row, so this can only
		// happen while the text marker is between redraws. Wait for a fresh
		// snapshot instead of sending an unrelated input.
		return 0, false
	}
}

// waitFleeMenu advances encounter text/animations until either battle menu
// that Flee understands is positively rendered. It stops before sending A on
// top of the menu itself.
func waitFleeMenu(m *emu.Emu) (fleeMenuKind, error) {
	start := m.FrameCount()
	for {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if kind := fleeMenuFromMem(&mem); kind != fleeMenuNone {
			return kind, nil
		}
		if int(m.FrameCount()-start) > bagMainMenuBudget {
			return fleeMenuNone, fmt.Errorf("skill: Flee: battle RUN menu did not open within %d frames", bagMainMenuBudget)
		}
		m.Tap(emu.A, 3, 7)
	}
}

// Flee attempts to escape a wild battle. A failed attempt is not an error:
// wNumRunAttempts (ram/wram.asm) improves the odds on each try within the
// same battle — core.asm TryRunningFromBattle adds 30 to the escape
// quotient per prior attempt — so Flee retries up to attempts times. The
// postcondition is POSITIVE — the battle is over (wIsInBattle clear, read
// back via DecodeBattle) and the player is controllable again; "the menu
// went away" is not enough, settleAfterBattle enforces it.
//
// Safari battles are also supported: they use BALL/BAIT/THROW ROCK/RUN rather
// than the normal battle menu, and the ROM's TryRunningFromBattle explicitly
// makes Safari RUN unconditional. Flee still verifies that the battle ended
// and the overworld became controllable.
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
	kind, err := waitFleeMenu(m)
	if err != nil {
		return 0, err
	}
	switch kind {
	case fleeMenuNormal:
		if err := selectRunEntry(m); err != nil {
			return 0, err
		}
	case fleeMenuSafari:
		if err := selectSafariRunEntry(m); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("skill: Flee: unsupported battle menu %d", kind)
	}
	m.Tap(emu.A, 3, 7)

	var mem state.Mem
	start := m.FrameCount()
	// refused remembers that the trainer has already rejected RUN. The ROM
	// does not return straight to the main menu after that refusal — per
	// ErrTrainerBattle's doc comment it sets wForcePlayerToChooseMon instead
	// — and MEASURED live (2026-08-31, checkpoint from the Viridian Forest
	// wedge) the very next screen is the voluntary switch menu ("Choose a
	// POKéMON."), not the FIGHT/ITEM/PKMN/RUN menu. Returning ErrTrainerBattle
	// the instant the refusal text appears — the old behaviour — orphaned
	// that screen: Battle()'s state machine has no case for it, so its
	// default (Tap A) picked SWITCH on the only, already-active party
	// member every time, and the ROM's "X is already out!" bounce sent it
	// right back, forever, until the 60000-frame cap. Now the refusal is
	// remembered and draining continues below until the battle is actually
	// back at a state Battle() can pick up.
	refused := false
	forcedChoiceRounds := 0
	for int(m.FrameCount()-start) < fleeAttemptBudget {
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) == nil {
			// Escaped: the battle is over. Settle the end-of-battle text and
			// wait until the player is controllable — the positive half of
			// the postcondition.
			return fleeSucceeded, settleAfterBattle(m, &mem)
		}
		switch {
		case !refused && strings.Contains(state.ScreenText(&mem), trainerNoRunningMarker):
			refused = true
			m.Tap(emu.A, 3, 7)

		case switchBoxUp(m):
			// SWITCH/STATS/CANCEL box (engine/battle/core.asm
			// .partyMonWasSelected — CONFIRMED against pret/pokered): B is
			// watched here and takes .partyMonDeselected — back to the
			// party list, not out of the battle. Bounded regardless.
			forcedChoiceRounds++
			if forcedChoiceRounds > forcedChoiceCap {
				x, y := playerXY(m)
				return 0, fmt.Errorf("skill: Flee: map %02x at (%d,%d): %w", m.Peek8(sym.CurMap), x, y, ErrForcedChoiceStuck)
			}
			m.Tap(emu.B, 3, 7)

		case battleSwitchMenuUp(m):
			// The forced choice the refusal above triggers. CONFIRMED
			// against pret/pokered (home/pokemon.asm PartyMenuInit): B is
			// disabled on the FIRST visit to this list
			// (wForcePlayerToChooseMon), so the only legal move is picking
			// the slot, which opens the box above. Backing out of that box
			// re-runs PartyMenuInit with the flag already cleared, so B
			// is watched on every visit after — and pressing it then
			// genuinely exits to the main battle menu.
			if forcedChoiceRounds == 0 {
				var s state.Mem
				state.Snapshot(m, &s)
				slot := firstLivePartySlot(&s)
				if slot < 0 {
					m.StepFrame()
					continue
				}
				if err := SelectPartySlot(m, slot); err != nil {
					return 0, fmt.Errorf("skill: Flee: select party slot (forced choice): %w", err)
				}
			} else {
				m.Tap(emu.B, 3, 7)
			}

		case mainMenuUp(m):
			if refused {
				x, y := playerXY(m)
				return 0, fmt.Errorf("skill: Flee: map %02x at (%d,%d): %w", m.Peek8(sym.CurMap), x, y, ErrTrainerBattle)
			}
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

// selectSafariRunEntry moves the Safari BALL/BAIT/THROW ROCK/RUN cursor to
// RUN using the menu's actual live row/X state. Every move is followed by a
// positive read-back before the next input.
func selectSafariRunEntry(m *emu.Emu) error {
	for i := 0; i < 12; i++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		btn, done := safariRunNextInput(&mem)
		if done {
			return nil
		}
		if btn == 0 {
			m.StepFrame()
			continue
		}
		prevX, prevRow := mem.U8(sym.TopMenuItemX), int(mem.U8(sym.CurrentMenuItem))
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return m.Peek8(sym.TopMenuItemX) != prevX || int(m.Peek8(sym.CurrentMenuItem)) != prevRow
		}); err != nil {
			return fmt.Errorf("skill: Flee: Safari cursor stuck at x=%#02x row %d, want RUN (x=%#02x row %d)",
				prevX, prevRow, safariBattleMenuRightX, mainMenuMax)
		}
	}
	return fmt.Errorf("skill: Flee: Safari cursor did not reach RUN")
}
