package agent

import (
	"strings"
	"testing"
)

func TestStrategicMemoryKeepsSpeculationSeparateAndBounded(t *testing.T) {
	var m StrategicMemory
	for i := 0; i < 10; i++ {
		m.AddHypothesis(string(rune('a' + i)))
	}
	if len(m.Hypotheses) != strategyHypothesisCap {
		t.Fatalf("hypotheses = %d, want %d", len(m.Hypotheses), strategyHypothesisCap)
	}
	m.AddHypothesis("h")
	if m.Hypotheses[0] != "h" {
		t.Fatalf("duplicate was not moved to front: %v", m.Hypotheses)
	}
}

func TestStrategicMemoryDetectsLongHorizonStall(t *testing.T) {
	var m StrategicMemory
	m.SetStrategy("train before trying the gym again")
	obs := Observation{Map: 1, Badges: []string{"boulder"}, Events: []string{"x"}, Party: []PartyMon{{Level: 12}}}
	if !m.ObserveProgress(obs, 5) {
		t.Fatal("first observation should establish progress baseline")
	}
	for i := 0; i < 4; i++ {
		if m.ObserveProgress(obs, 5) {
			t.Fatal("identical observation counted as progress")
		}
	}
	reason := m.ReplanReason(4)
	if !strings.Contains(reason, "materially different") {
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

func TestStrategyAgeOnlyAdvancesWhenUnchanged(t *testing.T) {
	var m StrategicMemory
	m.SetStrategy("explore north")
	m.SetStrategy("explore north")
	m.SetStrategy("explore north")
	if m.Age != 2 {
		t.Fatalf("Age = %d, want 2", m.Age)
	}
	m.SetStrategy("heal first")
	if m.Age != 0 || m.Strategy != "heal first" {
		t.Fatalf("strategy change = %+v", m)
	}
}
