package agent

import (
	"strings"
	"testing"
)

func TestStrategicMemoryDetectsLongHorizonStallUsingIntent(t *testing.T) {
	var m StrategicMemory
	obs := Observation{Map: 1, Badges: []string{"boulder"}, Events: []string{"x"}, Party: []PartyMon{{Level: 12}}}
	if !m.ObserveProgress(obs, 5) {
		t.Fatal("first observation should establish progress baseline")
	}
	for i := 0; i < 4; i++ {
		if m.ObserveProgress(obs, 5) {
			t.Fatal("identical observation counted as progress")
		}
	}
	reason := m.ReplanReason(4, "train before trying the gym again")
	if !strings.Contains(reason, `Intent "train before trying the gym again"`) || !strings.Contains(reason, "materially different") {
		t.Fatalf("replan reason = %q", reason)
	}
}

func TestStrategicMemoryCanSignalWithoutIntent(t *testing.T) {
	var m StrategicMemory
	obs := Observation{Map: 1, Party: []PartyMon{{Level: 12}}}
	m.ObserveProgress(obs, 1)
	m.ObserveProgress(obs, 1)
	if reason := m.ReplanReason(1, ""); !strings.Contains(reason, "No measurable progress for 1 rounds") {
		t.Fatalf("replan reason = %q", reason)
	}
}

func TestStrategicMemoryResetsStallOnMeasurableProgress(t *testing.T) {
	var m StrategicMemory
	obs := Observation{Map: 1, Party: []PartyMon{{Level: 12}}}
	m.ObserveProgress(obs, 1)
	m.ObserveProgress(obs, 1)
	m.ObserveProgress(obs, 1)
	if m.NoProgress != 2 {
		t.Fatalf("NoProgress = %d, want 2", m.NoProgress)
	}
	obs.Party[0].Level = 13
	if !m.ObserveProgress(obs, 1) || m.NoProgress != 0 {
		t.Fatalf("level gain did not reset stall: %+v", m)
	}
}

func TestReplanReasonWaitsForThreshold(t *testing.T) {
	m := StrategicMemory{NoProgress: 3}
	if got := m.ReplanReason(4, "explore north"); got != "" {
		t.Fatalf("early replan reason = %q", got)
	}
}
