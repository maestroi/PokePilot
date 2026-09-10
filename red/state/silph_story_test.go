package state

import "testing"

func TestDecodeSilphStoryFacts(t *testing.T) {
	var mem Mem
	setTestEvent(&mem, eventBeatSilphCoRival)
	setTestEvent(&mem, eventBeatSilphCoGiovanni)

	got := DecodeStoryFacts(&mem, InventoryState{})
	if !got.SilphCoRivalDefeated || !got.SilphCoCleared {
		t.Fatalf("Silph battle facts = %+v", got)
	}
	if got.MasterBallAwarded || got.SilphRescueComplete {
		t.Fatalf("Giovanni alone completed president reward: %+v", got)
	}

	setTestEvent(&mem, eventGotMasterBall)
	got = DecodeStoryFacts(&mem, InventoryState{})
	if !got.MasterBallAwarded || !got.SilphRescueComplete {
		t.Fatalf("Master Ball award did not complete Silph rescue: %+v", got)
	}
}

func TestSilphStoryEventIndicesMatchDecomp(t *testing.T) {
	events := parseEventConstants(t)
	pairs := []struct {
		label string
		event Event
	}{
		{"EVENT_BEAT_SILPH_CO_RIVAL", eventBeatSilphCoRival},
		{"EVENT_GOT_MASTER_BALL", eventGotMasterBall},
		{"EVENT_BEAT_SILPH_CO_GIOVANNI", eventBeatSilphCoGiovanni},
	}
	for _, pair := range pairs {
		want, ok := events[pair.label]
		if !ok {
			t.Errorf("label %s not found in %s", pair.label, eventConstantsFile)
			continue
		}
		if uint16(pair.event) != want {
			t.Errorf("%s = %#x, want %#x", pair.label, uint16(pair.event), want)
		}
	}
}
