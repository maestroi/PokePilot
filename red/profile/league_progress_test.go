package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

// Intermediate Elite Four stages must project through ProjectStoryFacts so
// ObserveChecked and adapter.Observe agree. Missing them made sealed League
// rooms re-offer an already-committed predecessor (run-a4wn4o17ztx91zgnezctug9t3).
func TestProjectStoryFactsIncludesLeagueStages(t *testing.T) {
	facts := state.StoryFacts{
		LeagueChallengeStarted: true,
		LeagueLoreleiDefeated:  true,
		LeagueBrunoDefeated:    true,
		LeagueAgathaDefeated:   true,
	}
	progress := ProjectStoryFacts(facts)
	for _, id := range []game.ProgressID{
		ProgressLeagueChallengeStarted,
		ProgressLeagueLoreleiDefeated,
		ProgressLeagueBrunoDefeated,
		ProgressLeagueAgathaDefeated,
	} {
		if !progress.Has(id) {
			t.Fatalf("ProjectStoryFacts missing %q: %v", id, progress)
		}
	}
	if progress.Has(ProgressLeagueLanceDefeated) {
		t.Fatal("Lance projected complete before its fact was set")
	}
}
