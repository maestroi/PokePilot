package skill

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// The Silph trigger loops used to replace every drive failure with one prose
// error, so a lost required battle and a stuck battle menu reached the agent as
// the same unknown_error and recovery replayed the same checkpoint into the
// same failure until the circuit opened (run-39etso0zuq4451wr2duvk128ph). The
// composed error must keep the typed cause.
func TestSilphTriggerFailureKeepsTypedCause(t *testing.T) {
	const message = "rival triggers completed without silph rival defeated fact"

	loss := &RequiredBattleError{Outcome: RequiredBattleOutcome{
		Encounter: "silph:Silph rival",
		Result:    state.ResultLost,
		Trainer:   true,
	}}
	got := silphTriggerFailure(message, fmt.Errorf("skill: ClearSilphCo: battle Silph rival: %w", loss))
	if !strings.Contains(got.Error(), message) {
		t.Fatalf("composed error %q lost the trigger context", got)
	}
	var required *RequiredBattleError
	if !errors.As(got, &required) || required.Outcome.Encounter != "silph:Silph rival" {
		t.Fatalf("composed error %q lost the required-battle outcome", got)
	}
	if !errors.Is(got, ErrTrainerBlackedOut) {
		t.Fatalf("composed error %q lost the blackout sentinel", got)
	}

	stuck := silphTriggerFailure(message, fmt.Errorf("skill: ClearSilphCo: battle Silph rival: select FIGHT: %w", ErrMenuStuck))
	if !errors.Is(stuck, ErrMenuStuck) {
		t.Fatalf("composed error %q lost the menu-stuck cause", stuck)
	}

	// No drive failure to report still names the exhausted triggers.
	if bare := silphTriggerFailure(message, nil); !strings.Contains(bare.Error(), message) {
		t.Fatalf("bare error %q lost the trigger context", bare)
	}
}

// A resolved required battle is durable: after a loss the player has already
// blacked out of the room, so the other candidate trigger tile cannot change
// the outcome and must not be retried.
func TestSilphDriveResolvedBattleOnlyForTypedOutcomes(t *testing.T) {
	loss := &RequiredBattleError{Outcome: RequiredBattleOutcome{Result: state.ResultLost, Trainer: true}}
	if !silphDriveResolvedBattle(fmt.Errorf("skill: ClearSilphCo: battle Silph rival: %w", loss)) {
		t.Fatal("a wrapped required-battle outcome was treated as retryable")
	}
	if silphDriveResolvedBattle(fmt.Errorf("select FIGHT: %w", ErrMenuStuck)) {
		t.Fatal("a stuck battle menu was treated as a resolved battle")
	}
	if silphDriveResolvedBattle(nil) {
		t.Fatal("nil was treated as a resolved battle")
	}
}
