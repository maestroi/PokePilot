package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestJourneyProgressionBlockedRoute3UntilBoulderBadge(t *testing.T) {
	obs := Observation{}
	if !journeyProgressionBlocked(obs, route3Map) {
		t.Fatal("Route 3 should be blocked before the Boulder Badge")
	}
	if journeyProgressionBlocked(obs, 0x0d) {
		t.Fatal("unrelated Route 2 journey should not be blocked")
	}

	obs.Badges = []string{state.BadgeBoulder.String()}
	if journeyProgressionBlocked(obs, route3Map) {
		t.Fatal("Route 3 should be available after the Boulder Badge")
	}
}
