package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
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
