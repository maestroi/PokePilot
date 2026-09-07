package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestJourneyProgressionBlockedRoute3UntilBoulderBadge(t *testing.T) {
	// The Pokedex is held: Route 2's own gate is open, so it is a control
	// for "the block is specific to this destination" rather than a second
	// blocked journey.
	obs := Observation{Events: []string{state.EventGotPokedex.String()}}
	if !journeyProgressionBlocked(obs, route3Map) {
		t.Fatal("Route 3 should be blocked before the Boulder Badge")
	}
	if journeyProgressionBlocked(obs, route2Map) {
		t.Fatal("unrelated Route 2 journey should not be blocked")
	}

	obs.Badges = []string{state.BadgeBoulder.String()}
	if journeyProgressionBlocked(obs, route3Map) {
		t.Fatal("Route 3 should be available after the Boulder Badge")
	}
}

func TestOfferSuppressesPewterRoute3UntilBoulderBadge(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{
		0x02: {route3Map},
	})
	known.SawMap(0x02)
	obs := Observation{
		Map:        0x02,
		MapName:    "PEWTER_CITY",
		X:          10,
		Y:          18,
		PartyCount: 1,
		Party:      []PartyMon{{Level: 10, HP: 30, MaxHP: 30}},
	}

	plain, flee := offeredJourneyTo(obs, known, "route 3")
	if plain || flee {
		t.Fatalf("pre-Boulder Route 3 = plain:%v flee:%v, want both suppressed", plain, flee)
	}

	obs.Badges = []string{state.BadgeBoulder.String()}
	plain, flee = offeredJourneyTo(obs, known, "route 3")
	if !plain || !flee {
		t.Fatalf("post-Boulder Route 3 = plain:%v flee:%v, want both offered", plain, flee)
	}
}

func TestJourneyProgressionBlockedRoute2UntilPokedex(t *testing.T) {
	// The Viridian guard blocks the north exit until Oak's parcel is
	// delivered; the Pokedex changing hands is that moment.
	obs := Observation{}
	if !journeyProgressionBlocked(obs, route2Map) {
		t.Fatal("Route 2 should be blocked before the parcel is delivered")
	}
	// Holding the parcel is not delivering it: the guard is still there.
	obs.Events = []string{state.EventGotOaksParcel.String()}
	if !journeyProgressionBlocked(obs, route2Map) {
		t.Fatal("Route 2 should stay blocked while the parcel is only carried")
	}
	obs.Events = append(obs.Events, state.EventGotPokedex.String())
	if journeyProgressionBlocked(obs, route2Map) {
		t.Fatal("Route 2 should open once the Pokedex is in hand")
	}
}
