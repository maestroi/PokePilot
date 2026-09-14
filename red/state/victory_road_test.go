package state

import "testing"

func TestVictoryRoadClearedUsesFinalEastSwitchEvent(t *testing.T) {
	var mem Mem
	if VictoryRoadCleared(&mem) {
		t.Fatal("fresh state reported Victory Road cleared")
	}
	setTestEvent(&mem, eventVictoryRoad2EastSwitch)
	if !VictoryRoadCleared(&mem) {
		t.Fatal("final 2F east-switch event did not report Victory Road cleared")
	}
}

func TestVictoryRoadClearEventMatchesDecomp(t *testing.T) {
	events := parseEventConstants(t)
	want, ok := events["EVENT_VICTORY_ROAD_2_BOULDER_ON_SWITCH2"]
	if !ok {
		t.Fatal("EVENT_VICTORY_ROAD_2_BOULDER_ON_SWITCH2 missing from event constants")
	}
	if uint16(eventVictoryRoad2EastSwitch) != want {
		t.Fatalf("Victory Road clear event = %#x, want %#x", uint16(eventVictoryRoad2EastSwitch), want)
	}
}
