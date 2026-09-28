package skill

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

func TestRoute22ResolvedCheckpointStillNeedsSettlement(t *testing.T) {
	facts := state.StoryFacts{Route22RivalResolved: true}
	if route22RivalCanReturnImmediately(facts) {
		t.Fatal("event-positive Route 22 rival checkpoint skipped trailing-script settlement")
	}

	facts.LeagueChallengeStarted = true
	if !route22RivalCanReturnImmediately(facts) {
		t.Fatal("League-started state should not replay Route 22 rival settlement")
	}
}

// TestVictoryRoadWestSwitchRecoveryIsNarrow locks the safety property of the
// exit-side recovery: a resumed checkpoint inside 2F's 3F-ladder pocket cannot
// reach the west boulders, so that one proven-unsolvable outcome is allowed to
// walk out to Indigo. Every other failure mode — and the ordinary entrance-side
// state where no ladder is reachable — must keep failing the objective instead
// of being silently swallowed.
func TestVictoryRoadWestSwitchRecoveryIsNarrow(t *testing.T) {
	unsolvable := fmt.Errorf("skill: boulder puzzle map 0xc2: %w", &world.PushPuzzleError{
		Kind: world.ErrPushPuzzleNoSolution, Explored: 1, MaxStates: 20000,
	})
	if !victoryRoadWestSwitchRecoverable(unsolvable, true) {
		t.Fatal("proven-unsolvable puzzle with a reachable ladder must recover to the exit side")
	}
	if victoryRoadWestSwitchRecoverable(unsolvable, false) {
		t.Fatal("no reachable ladder means the entrance side: the puzzle must still fail the objective")
	}

	// An exhausted search budget proves nothing about reachability, and a
	// battle interruption is a different problem entirely.
	limit := fmt.Errorf("skill: boulder puzzle map 0xc2: %w", &world.PushPuzzleError{
		Kind: world.ErrPushPuzzleStateLimit, Explored: 20000, MaxStates: 20000,
	})
	if victoryRoadWestSwitchRecoverable(limit, true) {
		t.Fatal("a state-limit exhaustion must not be treated as an unreachable puzzle")
	}
	if victoryRoadWestSwitchRecoverable(errors.New("battle in progress"), true) {
		t.Fatal("unrelated failures must not recover")
	}
}
