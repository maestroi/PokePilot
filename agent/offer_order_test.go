package agent

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// TestVerbsDoNotSinkAsTheWorldGrows pins the regression that made runs less
// consistent as the agent got better at exploring: journeys multiply with
// the number of known maps, and while they were listed first, every map the
// run discovered pushed the story verb further down a list the model reads
// top-down. MEASURED: "deliver oak's parcel" went from index 9 of 9 to
// index 17 of 17 purely because the errand's own walk got recorded.
func TestVerbsDoNotSinkAsTheWorldGrows(t *testing.T) {
	obs := Observation{
		Map: 0x28, MapName: "OAKS_LAB", X: 5, Y: 6, PartyCount: 1,
		Party:  []PartyMon{{Level: 5, HP: 19, MaxHP: 19}},
		Events: []string{state.EventBattledRivalInOaksLab.String()},
	}
	adj := map[uint8][]uint8{0x28: {0x00}, 0x00: {0x0c, 0x25, 0x28}, 0x0c: {0x00, 0x01}}

	indexOfErrand := func(visited ...uint8) (int, int) {
		k := NewKnowledge(adj)
		for _, m := range visited {
			k.SawMap(m)
		}
		offered := Offer(obs, k)
		for i, o := range offered {
			if o.Kind == KindErrand {
				return i + 1, len(offered)
			}
		}
		t.Fatal("the errand is not offered at all")
		return 0, 0
	}

	small, smallLen := indexOfErrand(0x26, 0x25, 0x00, 0x28)
	big, bigLen := indexOfErrand(0x26, 0x25, 0x00, 0x28, 0x0c, 0x01, 0x2a, 0x29)
	if bigLen <= smallLen {
		t.Fatalf("setup: the bigger world offered %d, not more than %d", bigLen, smallLen)
	}
	if big != small {
		t.Fatalf("the errand moved from index %d to %d as the world grew: a verb must not sink behind the travel list", small, big)
	}

	// And the tail really is the journeys, so nothing else can drift above.
	k := NewKnowledge(adj)
	for _, m := range []uint8{0x26, 0x25, 0x00, 0x28, 0x0c, 0x01} {
		k.SawMap(m)
	}
	offered := Offer(obs, k)
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
	if !strings.Contains(offered[0].String(), "parcel") {
		t.Fatalf("first offer = %q, want the verb", offered[0])
	}
}

// TestMenuCarriesItsOwnHistory: the counts already existed, in Failures and
// Completed, and a model reading a numbered list top-down skipped them. They
// now ride on the line being chosen. Objective-local factual notes may share
// that line; history composes with them instead of replacing them. String()
// is untouched, so an annotated objective is still the same objective
// everywhere it is identified by name.
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
		// The note must never leak into identity: Completed and Failures are
		// keyed by String(), and a note in there would orphan every count.
		if strings.Contains(o.String(), "(") && strings.Contains(o.String(), "x)") {
			t.Fatalf("String() carries the note: %q", o)
		}
	}
	if !sawLab || !sawRoute {
		t.Fatal("the annotated objectives were not offered at all")
	}
}
