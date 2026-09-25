package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/sym"
)

// InterruptibleAction is one bounded, resumable piece of precise world work
// (an exact warp, a dungeon drop, a boulder push sequence, facing an item)
// that an incidental battle or text box may interrupt.
//
// Run owns the action and its positive completion check. It returns nil only
// once the world positively shows the action finished, and it returns the
// movement layer's interruption sentinels (ErrBattleInterrupted, ErrBattle,
// ErrDialogueInterrupted) unchanged when control was stolen. After every
// resolved interruption Run is called again from scratch: the battle or box
// may have moved the player, changed the map, or let a pending warp finish,
// so Run must re-observe live state and re-plan instead of resuming a path
// or stand tile computed before the interruption.
type InterruptibleAction struct {
	// Name labels structured errors and telemetry for the original action.
	Name string
	// MaxEngagements bounds battles fought or fled while the action is
	// interrupted. It must be > 0; there is no "unlimited".
	MaxEngagements int
	Run            func() error
}

// RunInterruptible executes a bounded local action under the same
// interruption contract Travel owns for whole journeys, so skills provide the
// action and its postcondition while this shared layer owns recovery:
//
//   - wild battles are fled (guaranteed flee attempts); trainer battles, which
//     cannot be fled, are fought with policy;
//   - a lost battle or a poison blackout ends the action with ErrBlackedOut
//     (ErrTrainerBlackedOut / RequiredBattleError for trainers), the shared
//     combat-loss contract, never a silent retry;
//   - ordinary text boxes are paged closed by RecoverDialogue, which never
//     answers a choice: an unanswered choice comes back as *ErrDialogueChoice
//     and an open menu the same way, so unknown prompts fail closed;
//   - engagements, dialogue recoveries and same-box repeats are bounded, so an
//     action that keeps getting interrupted terminates deterministically with
//     ErrEngagementsExhausted or a looping-box error instead of spinning.
//
// A nil policy means the caller owns interruptions: Run is attempted once and
// its error, interruption sentinels included, is returned unchanged. That is
// the plain-GoTo contract, which never fights from inside pathing.
func RunInterruptible(m *emu.Emu, policy MovePolicy, a InterruptibleAction) (TravelResult, error) {
	if a.Run == nil {
		return TravelResult{}, fmt.Errorf("skill: RunInterruptible %q: nil action", a.Name)
	}
	if policy == nil {
		return TravelResult{}, a.Run()
	}
	if a.MaxEngagements <= 0 {
		return TravelResult{}, fmt.Errorf("skill: RunInterruptible %q: MaxEngagements must be > 0, got %d", a.Name, a.MaxEngagements)
	}
	return runInterruptions(m, a.MaxEngagements, a.Run, interruptionResolvers{
		label:         a.Name,
		recoverBox:    func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		blackout:      func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		resolveBattle: fleeThenFight(m, policy, guaranteedWildFleeAttempts),
	})
}

// interruptionResolvers are the pieces of runInterruptions that touch the
// emulator. Tests replace them with fakes; observe and settle default to the
// live RAM readers.
type interruptionResolvers struct {
	label         string
	recoverBox    func() DialogueRecoveryResult
	blackout      func() bool
	resolveBattle resolveBattle
	// observe reads the pre-battle world; settle waits for the post-battle
	// world (including a blackout respawn) to stand still and returns it.
	observe func() (Replan, error)
	settle  func(pre Replan, lost bool) (Replan, error)
}

func (r interruptionResolvers) withWorldDefaults(m *emu.Emu) (interruptionResolvers, error) {
	if r.observe != nil && r.settle != nil {
		return r, nil
	}
	decoder, err := overworldDecoderFor(m)
	if err != nil {
		return r, err
	}
	if r.observe == nil {
		r.observe = func() (Replan, error) { return currentWorldWithDecoder(m, decoder) }
	}
	if r.settle == nil {
		r.settle = func(pre Replan, lost bool) (Replan, error) {
			return settleWorldWithDecoder(m, decoder, pre, lost)
		}
	}
	return r, nil
}

// normalizeInterruption folds the movement layer's battle sentinel into the
// one the interruption loop resolves. WalkPath/StepOnce report
// ErrBattleInterrupted; GoTo reports ErrBattle. Both mean a live battle owns
// the screen, and callers used to hand-translate between them before every
// Travel-style recovery. The original error stays in the chain so its
// context survives into telemetry.
func normalizeInterruption(err error) error {
	if err == nil || errors.Is(err, ErrBattle) || !errors.Is(err, ErrBattleInterrupted) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrBattle, err)
}
