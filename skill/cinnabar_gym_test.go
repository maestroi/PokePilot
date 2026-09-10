package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func setSkillTestEvent(mem *state.Mem, event state.Event) {
	addr := sym.EventFlags + uint16(event)/8
	mem[addr] |= 1 << (uint16(event) % 8)
}

func TestCinnabarQuizSpecsMatchROMHiddenEvents(t *testing.T) {
	want := []cinnabarQuizSpec{
		{Index: 1, TargetX: 15, TargetY: 7, CorrectAnswer: false},
		{Index: 2, TargetX: 10, TargetY: 1, CorrectAnswer: true},
		{Index: 3, TargetX: 9, TargetY: 7, CorrectAnswer: true},
		{Index: 4, TargetX: 9, TargetY: 13, CorrectAnswer: true},
		{Index: 5, TargetX: 1, TargetY: 13, CorrectAnswer: false},
		{Index: 6, TargetX: 1, TargetY: 7, CorrectAnswer: true},
	}
	if len(cinnabarQuizSpecs) != len(want) {
		t.Fatalf("quiz count = %d, want %d", len(cinnabarQuizSpecs), len(want))
	}
	for i, got := range cinnabarQuizSpecs {
		if got != want[i] {
			t.Fatalf("quiz %d = %+v, want %+v", i+1, got, want[i])
		}
		if got.TargetY+1 == got.TargetY {
			t.Fatalf("quiz %d has invalid south-side stand", got.Index)
		}
	}
}

func TestCinnabarQuizGateEventsAreTheSixLiveGateBits(t *testing.T) {
	for index := uint8(1); index <= 6; index++ {
		event, ok := cinnabarQuizGateEvent(index)
		if !ok {
			t.Fatalf("gate %d has no event", index)
		}
		want := state.Event(0x2a8) + state.Event(index)
		if event != want {
			t.Fatalf("gate %d event = %#x, want %#x", index, uint16(event), uint16(want))
		}
	}
	if _, ok := cinnabarQuizGateEvent(0); ok {
		t.Fatal("gate index 0 unexpectedly accepted as a quiz gate")
	}
	if _, ok := cinnabarQuizGateEvent(7); ok {
		t.Fatal("gate index 7 unexpectedly accepted")
	}
}

func TestCinnabarGymReadyRequiresSecretKey(t *testing.T) {
	var mem state.Mem
	if CinnabarGymReady(&mem) {
		t.Fatal("Cinnabar Gym ready without Secret Key")
	}
	putBag(&mem, state.BagItem{ID: mansionSecretKeyItem, Quantity: 1})
	if !CinnabarGymReady(&mem) {
		t.Fatal("Cinnabar Gym not ready with Secret Key in bag")
	}
}

func TestCinnabarGymOpenRequiresAllSixQuizGates(t *testing.T) {
	var mem state.Mem
	for index := uint8(1); index <= 6; index++ {
		event, _ := cinnabarQuizGateEvent(index)
		setSkillTestEvent(&mem, event)
		if index < 6 && CinnabarGymOpen(&mem) {
			t.Fatalf("gym reported open after only %d gates", index)
		}
	}
	if !CinnabarGymOpen(&mem) {
		t.Fatal("gym not open after all six quiz gate events")
	}
}

func TestBlaineGymRegistration(t *testing.T) {
	g, ok := GymAt(cinnabarGymMap)
	if !ok {
		t.Fatal("Cinnabar Gym is not registered")
	}
	if g.Leader != "BLAINE" || g.Badge != state.BadgeVolcano || g.LeaderX != 3 || g.LeaderY != 3 {
		t.Fatalf("Cinnabar Gym registration = %+v", g)
	}
	dest, ok := Place("cinnabar gym")
	if !ok {
		t.Fatal("cinnabar gym place missing")
	}
	if dest.Map != cinnabarGymMap || dest.X != 3 || dest.Y != 4 {
		t.Fatalf("cinnabar gym destination = %+v, want a6(3,4)", dest)
	}
}
