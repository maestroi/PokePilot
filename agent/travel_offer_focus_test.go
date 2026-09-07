package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// A visited map stays an executable destination from wherever the player
// stands: Travel routes across maps, and the run that could only see its
// immediate neighbours walked Viridian <-> Route 2 for hundreds of rounds
// with Route 3 — visited, one map past Pewter — off the menu entirely.
func TestOfferKeepsVisitedDestinationsBeyondTheCurrentMap(t *testing.T) {
	here := mustPlaceForTravelFocusTest(t, "viridian city")
	far := mustPlaceForTravelFocusTest(t, "route 3")

	known := NewKnowledge(map[uint8][]uint8{here.Map: {0x0C}})
	known.Visited[far.Map] = true
	obs := Observation{
		Map:    here.Map,
		X:      here.X,
		Y:      here.Y,
		Badges: []string{state.BadgeBoulder.String()}, // Route 3's scripted gate
	}

	for _, o := range Offer(obs, known) {
		if o.Kind == KindGoTo && o.Place == "route 3" {
			return
		}
	}
	t.Fatal("a visited map two hops away is not offered as a destination")
}

func mustPlaceForTravelFocusTest(t *testing.T, name string) skill.Destination {
	t.Helper()
	destination, ok := skill.Place(name)
	if !ok {
		t.Fatalf("test place %q missing from skill table", name)
	}
	return destination
}
