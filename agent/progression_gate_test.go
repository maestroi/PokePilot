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
