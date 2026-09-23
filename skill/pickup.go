package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ErrPickupApproachIncomplete means navigation returned without leaving the
// player beside the item. Pickup must not interact from that position.
var ErrPickupApproachIncomplete = errors.New("skill: Pickup: approach did not reach the item")

// approachViaTravel walks beside the item, fleeing wild battles that interrupt
// the way. A trainer battle may move a sprite onto the chosen standing tile,
// so the destination must remain an interaction target: Travel then chooses a
// new side from live state after the battle. It is a no-op when already beside.
func approachViaTravel(m *emu.Emu, romData []byte, targetX, targetY uint8, policy MovePolicy) error {
	mapID := m.Peek8(sym.CurMap)
	x, y := playerXY(m)
	if _, adjacent := directionTo(x, y, targetX, targetY); adjacent {
		return nil
	}
	dest := InteractionDestination(mapID, targetX, targetY)
	_, err := TravelFlee(m, romData, dest, policy, 20)
	if errors.Is(err, ErrForcedChoiceStuck) {
		// A trainer can interrupt a forest/item approach after a failed RUN
		// attempt and leave Battle on the forced party-choice screen. The
		// battle state is still live, so throwing away the whole Pickup (and
		// eventually the whole run) is too harsh. Settle that one known menu
		// transition, finish the battle, then resume the same deterministic
		// approach from wherever the fight left the player.
		if recoverErr := recoverForcedChoiceBattle(m, policy); recoverErr != nil {
			return fmt.Errorf("skill: Pickup: recover trainer battle while approaching (%d,%d): %w", targetX, targetY, recoverErr)
		}
		_, err = TravelFlee(m, romData, dest, policy, 20)
	}
	if err != nil {
		return fmt.Errorf("skill: Pickup: approach beside (%d,%d) on map %#04x: %w", targetX, targetY, dest.Map, err)
	}
	x, y = playerXY(m)
	if _, adjacent := directionTo(x, y, targetX, targetY); m.Peek8(sym.CurMap) != mapID || !adjacent {
		return fmt.Errorf("%w: item on map %#04x at (%d,%d), player on map %#04x at (%d,%d)",
			ErrPickupApproachIncomplete, mapID, targetX, targetY, m.Peek8(sym.CurMap), x, y)
	}
	return nil
}

const pickupFaceRecoveryAttempts = 3

// pickupInterruptionForTravel translates the movement layer's battle sentinel
// into the one Travel's resolver owns. Dialogue already uses the same sentinel
// on both layers.
func pickupInterruptionForTravel(err error) error {
	if errors.Is(err, ErrBattleInterrupted) {
		return ErrBattle
	}
	return err
}

// recoverPickupInteractionInterruption owns the tiny race after an approach
// has finished but before Pickup presses A. A sighted trainer (or an ordinary
// wild encounter that lands on the last approach step) can take control in
// exactly that window: Face then times out because direction input is ignored.
// Run the same bounded dialogue/battle resolver used by TravelFlee, then let
// Pickup re-approach the ball and try the interaction again.
func recoverPickupInteractionInterruption(m *emu.Emu, policy MovePolicy) error {
	_, err := travel(
		m,
		policy,
		4,
		func() error { return pickupInterruptionForTravel(movementInterruption(m)) },
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		fleeThenFight(m, policy, guaranteedWildFleeAttempts),
	)
	return err
}

// ErrBagNotRisen reports that pressing A did not collect the wanted item:
// the bag count for it is not exactly one higher than before. A ball that
// was already collected fails here, cleanly — there is no event flag for
// ground items (verified: red/state decodes eight story events and none is
// a pickup), so a vanished ball has no data source to explain it, and the
// bag postcondition is the whole proof.
var ErrBagNotRisen = errors.New("skill: Pickup: bag count did not rise")

// ErrPickupMenu reports that a two-option menu appeared while paging the
// pickup text. A blind A into a yes/no prompt has cost this project a caught
// Caterpie (S6-3) and a learned move (S6-4); meeting one here is a finding
// to report, not a case to handle — no ground item in Red asks a question.
var ErrPickupMenu = errors.New("skill: Pickup: a two-option menu appeared while paging the pickup text")

// Pickup walks to the tile adjacent to an item ball, faces it and takes it.
// The postcondition is the bag: Pickup succeeds only when the count for want
// rose. A text box opening is not evidence.
//
// A small ROM-owned exception exists for scripted NPC item rewards (the three
// fishing gurus and Oak's aides). Those interactions deliberately reuse the
// same semantic objective/postcondition as Pickup, but their YES choice is
// owned by receiveChoiceReward rather than this generic item-ball path.
//
// The approach uses TravelFlee, not Approach: ground items sit in tall grass
// (the forest's antidote), and Approach aborts on the first wild battle by
// design — a pickup objective there would fail on every retry. Wild encounters
// are logistics noise for an item pickup, so they are fled; trainer battles,
// which cannot be fled, are fought with policy. A trainer blackout ends the
// approach as ErrBlackedOut, which is a recoverable outcome for the caller,
// not a dead end.
//
// Immediately before touching the ball, Pickup also guarantees capacity for
// a new distinct stack. That makes story-critical ground items such as Gold
// Teeth safe from the Gen I 20-stack bag limit without teaching their story
// meaning to this generic primitive. Existing stacks need no free slot.
func Pickup(m *emu.Emu, romData []byte, x, y uint8, want uint8, policy MovePolicy) error {
	if reward, ok := choiceRewardAt(m.Peek8(sym.CurMap), x, y, want); ok {
		return receiveChoiceReward(m, romData, reward, policy)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	before := bagCount(state.DecodeInventory(&mem).Items, want)

	if err := approachViaTravel(m, romData, x, y, policy); err != nil {
		return err
	}
	if err := EnsureBagSpaceFor(m, want); err != nil {
		return fmt.Errorf("skill: Pickup: make room for item %#02x: %w", want, err)
	}
	var faceErr error
	for attempt := 1; attempt <= pickupFaceRecoveryAttempts; attempt++ {
		faceErr = Face(m, x, y)
		if faceErr == nil {
			break
		}

		// If the face failed while the overworld is still idle, this is a
		// genuine interaction/controller failure. Recovery is only justified
		// when live RAM proves a battle/dialogue stole control after the
		// approach completed.
		if movementInterruption(m) == nil {
			return fmt.Errorf("skill: Pickup: %w", faceErr)
		}
		if attempt == pickupFaceRecoveryAttempts {
			return fmt.Errorf("skill: Pickup: face item at (%d,%d) still interrupted after %d recoveries: %w",
				x, y, pickupFaceRecoveryAttempts-1, faceErr)
		}
		if err := recoverPickupInteractionInterruption(m, policy); err != nil {
			return fmt.Errorf("skill: Pickup: recover interruption before facing item at (%d,%d): %w", x, y, err)
		}
		// A trainer fight can move the player a tile and a blackout can move
		// maps entirely. Re-establish the normal Pickup approach rather than
		// assuming the pre-interruption position is still valid.
		if err := approachViaTravel(m, romData, x, y, policy); err != nil {
			return err
		}
	}

	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(talkOpenBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0
	}); err != nil {
		return fmt.Errorf("skill: Pickup: A at (%d,%d) opened no text box: %w", x, y, ErrNoDialogue)
	}

	// Page the box closed. Before every A, check for a two-option menu and
	// STOP if one is up: the first loop pass checks the box the first press
	// opened, so a reflex A never answers a question on this path.
	for m.Peek8(sym.FontLoaded) != 0 {
		state.Snapshot(m, &mem)
		if menu := state.DecodeTwoOptionMenu(&mem); menu != nil {
			return fmt.Errorf("%w (cursor on option %d)", ErrPickupMenu, menu.Index)
		}
		m.Tap(emu.A, 3, 7)
		m.StepFrames(talkSettle)
	}

	// Same settle as Talk: the box is down, but wJoyIgnore may clear a few
	// frames after wFontLoaded.
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		if _, err := m.StepUntil(talkSettle, func(m *emu.Emu) bool {
			state.Snapshot(m, &mem)
			return state.Controllable(&mem)
		}); err != nil {
			return fmt.Errorf("skill: Pickup: not controllable %d frames after the box closed", talkSettle)
		}
	}

	state.Snapshot(m, &mem)
	after := bagCount(state.DecodeInventory(&mem).Items, want)
	if after != before+1 {
		return fmt.Errorf("%w: item %d was %d before and %d after", ErrBagNotRisen, want, before, after)
	}
	return nil
}
