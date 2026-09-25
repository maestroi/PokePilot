package skill

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// fakeInterruptionWorld drives runInterruptions without an emulator. world is
// what a live RAM read would observe; battles can move it, exactly as a
// trainer fight or a blackout respawn moves the player.
type fakeInterruptionWorld struct {
	world    Replan
	battles  int
	boxes    int
	onBattle func(n int) (battleResolution, error)
	onBox    func(n int) DialogueRecoveryResult
	blackout bool
}

func (f *fakeInterruptionWorld) resolvers(label string) interruptionResolvers {
	return interruptionResolvers{
		label: label,
		recoverBox: func() DialogueRecoveryResult {
			f.boxes++
			if f.onBox == nil {
				return DialogueRecoveryResult{Stop: DialogueRecovered}
			}
			return f.onBox(f.boxes)
		},
		blackout: func() bool { return f.blackout },
		resolveBattle: func() (battleResolution, error) {
			f.battles++
			if f.onBattle == nil {
				return battleResolution{fled: true}, nil
			}
			return f.onBattle(f.battles)
		},
		observe: func() (Replan, error) { return f.world, nil },
		settle:  func(Replan, bool) (Replan, error) { return f.world, nil },
	}
}

// An interruption that moves the player must make the action decide again
// from the new live state. The runner re-enters the action rather than
// resuming anything computed before the battle; the action proves it planned
// from fresh state by observing the post-battle tile.
func TestRunInterruptionsReentersActionFromPostInterruptionState(t *testing.T) {
	f := &fakeInterruptionWorld{world: Replan{Map: 0xc2, X: 10, Y: 10}}
	f.onBattle = func(int) (battleResolution, error) {
		// A trainer fight ends with the player shifted a tile.
		f.world = Replan{Map: 0xc2, X: 11, Y: 10}
		return battleResolution{outcome: state.ResultWon, trainer: true}, nil
	}
	var plannedFrom []Replan
	action := func() error {
		plannedFrom = append(plannedFrom, f.world)
		if len(plannedFrom) == 1 {
			// WalkPath's sentinel, not GoTo's: the runner owns the translation.
			return fmt.Errorf("walk to push stand: %w", ErrBattleInterrupted)
		}
		return nil
	}

	res, err := runInterruptions(nil, 5, action, f.resolvers("boulder puzzle"))
	if err != nil {
		t.Fatalf("runInterruptions: %v", err)
	}
	if len(plannedFrom) != 2 {
		t.Fatalf("action ran %d times, want 2 (re-entered after the battle)", len(plannedFrom))
	}
	if plannedFrom[1] != (Replan{Map: 0xc2, X: 11, Y: 10}) {
		t.Fatalf("re-entered action observed %+v, want the post-battle tile", plannedFrom[1])
	}
	if res.Battles != 1 || len(res.Replans) != 1 || res.Replans[0] != plannedFrom[1] {
		t.Fatalf("result = %+v, want one fought battle settled at the new tile", res)
	}
}

// A map change during recovery (a pending warp finishing under a text box)
// is also fresh state: the second attempt sees the new map, which is how the
// Mansion actions accept an already-completed warp as their postcondition.
func TestRunInterruptionsReentersActionAfterDialogueMapChange(t *testing.T) {
	f := &fakeInterruptionWorld{world: Replan{Map: 0x08, X: 3, Y: 3}}
	f.onBox = func(int) DialogueRecoveryResult {
		f.world = Replan{Map: 0xa5, X: 4, Y: 27}
		return DialogueRecoveryResult{Stop: DialogueRecovered, LastText: "sign"}
	}
	calls := 0
	action := func() error {
		calls++
		if f.world.Map == 0xa5 {
			return nil // positively observed landing
		}
		return ErrDialogueInterrupted
	}
	res, err := runInterruptions(nil, 5, action, f.resolvers("Mansion entrance"))
	if err != nil || calls != 2 || res.Dialogues != 1 {
		t.Fatalf("err=%v calls=%d dialogues=%d, want nil/2/1", err, calls, res.Dialogues)
	}
}

// A trainer loss exits through the shared combat-loss contract and is never
// retried: the caller learns the party blacked out and replans from there.
func TestRunInterruptionsTrainerDefeatUsesSharedCombatLossContract(t *testing.T) {
	f := &fakeInterruptionWorld{world: Replan{Map: 0x6c, X: 5, Y: 5}}
	f.onBattle = func(int) (battleResolution, error) {
		f.world = Replan{Map: 0x3a, X: 3, Y: 3} // respawn
		return battleResolution{outcome: state.ResultLost, trainer: true}, nil
	}
	calls := 0
	res, err := runInterruptions(nil, 5, func() error { calls++; return ErrBattleInterrupted }, f.resolvers("boulder puzzle"))
	if !errors.Is(err, ErrTrainerBlackedOut) || !errors.Is(err, ErrBlackedOut) {
		t.Fatalf("err = %v, want ErrTrainerBlackedOut (and ErrBlackedOut)", err)
	}
	var required *RequiredBattleError
	if !errors.As(err, &required) || !required.Outcome.Trainer {
		t.Fatalf("err = %v, want structured trainer RequiredBattleError", err)
	}
	if calls != 1 || !res.BlackedOut || !res.TrainerDefeat {
		t.Fatalf("calls=%d result=%+v, want one attempt ending in a trainer blackout", calls, res)
	}
}

// A wild loss is a plain blackout, also not retried.
func TestRunInterruptionsWildDefeatBlacksOut(t *testing.T) {
	f := &fakeInterruptionWorld{}
	f.onBattle = func(int) (battleResolution, error) {
		return battleResolution{outcome: state.ResultLost}, nil
	}
	calls := 0
	res, err := runInterruptions(nil, 5, func() error { calls++; return ErrBattleInterrupted }, f.resolvers("Pickup interaction"))
	if !errors.Is(err, ErrBlackedOut) || errors.Is(err, ErrTrainerBlackedOut) {
		t.Fatalf("err = %v, want ErrBlackedOut only", err)
	}
	if calls != 1 || !res.BlackedOut || res.TrainerDefeat {
		t.Fatalf("calls=%d result=%+v", calls, res)
	}
}

// An unanswered two-option prompt fails closed as a typed choice outcome; the
// runner neither answers it nor re-enters the action into the same box.
func TestRunInterruptionsUnknownChoiceFailsClosed(t *testing.T) {
	f := &fakeInterruptionWorld{}
	f.onBox = func(int) DialogueRecoveryResult {
		return DialogueRecoveryResult{Stop: DialogueChoiceRequired, Text: "Will you take it?"}
	}
	calls := 0
	_, err := runInterruptions(nil, 5, func() error { calls++; return ErrDialogueInterrupted }, f.resolvers("boulder puzzle"))
	var choice *ErrDialogueChoice
	if !errors.As(err, &choice) || choice.Result.Text != "Will you take it?" {
		t.Fatalf("err = %v, want *ErrDialogueChoice carrying the prompt", err)
	}
	if calls != 1 {
		t.Fatalf("action ran %d times, want 1 (a choice is never retried)", calls)
	}
}

// Endless interruption terminates at the configured budget with a typed,
// labelled diagnostic, and the number of attempts is exactly determined by
// that budget.
func TestRunInterruptionsTerminatesAtDeterministicEngagementBudget(t *testing.T) {
	const budget = 3
	for run := 0; run < 2; run++ {
		f := &fakeInterruptionWorld{}
		calls := 0
		res, err := runInterruptions(nil, budget, func() error { calls++; return ErrBattleInterrupted }, f.resolvers("Mansion 3F drop"))
		if !errors.Is(err, ErrEngagementsExhausted) {
			t.Fatalf("err = %v, want ErrEngagementsExhausted", err)
		}
		if !strings.Contains(err.Error(), "skill: Mansion 3F drop:") {
			t.Fatalf("err = %q, want the action label for telemetry", err)
		}
		if calls != budget+1 || res.Flees != budget || f.battles != budget {
			t.Fatalf("calls=%d flees=%d battles=%d, want %d/%d/%d", calls, res.Flees, f.battles, budget+1, budget, budget)
		}
	}
}

// Non-interruption failures keep their original typed error and are not
// retried: a genuine controller/navigation defect must not loop.
func TestRunInterruptionsPreservesNonInterruptionFailure(t *testing.T) {
	errDefect := errors.New("drop hole not adjacent")
	calls := 0
	_, err := runInterruptions(nil, 5, func() error { calls++; return errDefect }, (&fakeInterruptionWorld{}).resolvers("Mansion 3F drop"))
	if err != errDefect || calls != 1 {
		t.Fatalf("err=%v calls=%d, want the original error after one attempt", err, calls)
	}
}

// A text box that closes on a poison blackout ends the action as a blackout.
func TestRunInterruptionsDialogueBlackoutEndsAction(t *testing.T) {
	f := &fakeInterruptionWorld{blackout: true}
	calls := 0
	res, err := runInterruptions(nil, 5, func() error { calls++; return ErrDialogueInterrupted }, f.resolvers("Pickup interaction"))
	if !errors.Is(err, ErrBlackedOut) || calls != 1 || !res.BlackedOut {
		t.Fatalf("err=%v calls=%d result=%+v, want one attempt ending in ErrBlackedOut", err, calls, res)
	}
}

func TestRunInterruptibleValidatesBudgetAndAction(t *testing.T) {
	policy := MovePolicy(FirstUsableMove)
	if _, err := RunInterruptible(nil, policy, InterruptibleAction{Name: "x", Run: func() error { return nil }}); err == nil {
		t.Fatal("zero MaxEngagements accepted, want an error (no unlimited budget)")
	}
	if _, err := RunInterruptible(nil, policy, InterruptibleAction{Name: "x", MaxEngagements: 1}); err == nil {
		t.Fatal("nil Run accepted, want an error")
	}
}

func TestNormalizeInterruption(t *testing.T) {
	if normalizeInterruption(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	walk := fmt.Errorf("walk: %w", ErrBattleInterrupted)
	got := normalizeInterruption(walk)
	if !errors.Is(got, ErrBattle) || !errors.Is(got, ErrBattleInterrupted) {
		t.Fatalf("normalize(%v) = %v, want ErrBattle with the original chain kept", walk, got)
	}
	if got := normalizeInterruption(ErrDialogueInterrupted); got != ErrDialogueInterrupted {
		t.Fatalf("dialogue interruption changed to %v", got)
	}
	if got := normalizeInterruption(ErrBattle); got != ErrBattle {
		t.Fatalf("ErrBattle changed to %v", got)
	}
}
