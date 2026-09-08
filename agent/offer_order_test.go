package agent

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestVerbsDoNotSinkAsTheWorldGrows(t *testing.T) {
	obs := Observation{
		Map: 0x28, MapName: "OAKS_LAB", X: 5, Y: 6, PartyCount: 1,
		Party:  []PartyMon{{Level: 5, HP: 19, MaxHP: 19}},
		Events: []string{state.EventBattledRivalInOaksLab.String()},
	}
	adj := map[uint8][]uint8{0x28: {0x00}, 0x00: {0x0c, 0x25, 0x28}, 0x0c: {0x00, 0x01}}
	planner := &redObjectiveAdapter{}

	indexOfProgression := func(visited ...uint8) (int, int) {
		k := NewKnowledge(adj)
		for _, m := range visited {
			k.SawMap(m)
		}
		offered := OfferWithProgression(obs, k, planner)
		for i, o := range offered {
			if o.Kind == KindProgress && o.Progress == redProgressPokedexAcquired {
				return i + 1, len(offered)
			}
		}
		t.Fatal("Pokedex progression is not offered at all")
		return 0, 0
	}

	small, smallLen := indexOfProgression(0x26, 0x25, 0x00, 0x28)
	big, bigLen := indexOfProgression(0x26, 0x25, 0x00, 0x28, 0x0c, 0x01, 0x2a, 0x29)
	if bigLen <= smallLen {
		t.Fatalf("setup: the bigger world offered %d, not more than %d", bigLen, smallLen)
	}
	if big != small {
		t.Fatalf("progression moved from index %d to %d as the world grew", small, big)
	}

	k := NewKnowledge(adj)
	for _, m := range []uint8{0x26, 0x25, 0x00, 0x28, 0x0c, 0x01} {
		k.SawMap(m)
	}
	offered := OfferWithProgression(obs, k, planner)
	seenJourney := false
	for _, o := range offered {
		if o.Kind == KindGoTo {
			seenJourney = true
			continue
		}
		if seenJourney {
			t.Fatalf("%q comes after a journey; journeys must be last", o)
		}
	}
	if offered[0].Kind != KindProgress || offered[0].Progress != redProgressPokedexAcquired {
		t.Fatalf("first offer = %v, want Pokedex progression", offered[0])
	}
}

func TestMenuCarriesItsOwnHistory(t *testing.T) {
	obs := Observation{
		Map: 0x00, MapName: "PALLET_TOWN", X: 4, Y: 7, PartyCount: 1,
		Party:  []PartyMon{{Level: 5, HP: 19, MaxHP: 19}},
		Events: []string{state.EventBattledRivalInOaksLab.String()},
	}
	k := NewKnowledge(map[uint8][]uint8{0x00: {0x0c}})
	k.SawMap(0x00)

	lab := Objective{Kind: KindGoTo, Place: "oak's lab"}
	k.SawMap(0x28)
	k.Done(lab)
	k.Done(lab)
	k.Done(lab)
	route1 := Objective{Kind: KindGoTo, Place: "route 1"}
	k.Failed(route1, errors.New("blocked"))

	var sawLab, sawRoute bool
	for _, o := range Offer(obs, k) {
		switch o.String() {
		case lab.String():
			sawLab = true
			if o.Note != "(done 3x)" {
				t.Errorf("lab note = %q, want (done 3x)", o.Note)
			}
		case route1.String():
			sawRoute = true
			want := "(unvisited adjacent map) (failed 1x)"
			if o.Note != want {
				t.Errorf("route 1 note = %q, want %q", o.Note, want)
			}
		}
		if strings.Contains(o.String(), "(") && strings.Contains(o.String(), "x)") {
			t.Fatalf("String() carries the note: %q", o)
		}
	}
	if !sawLab || !sawRoute {
		t.Fatal("the annotated objectives were not offered at all")
	}
}
